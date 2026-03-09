package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"modelgate/internal/models"
)

type Adapter interface {
	ChatCompletion(ctx context.Context, req OpenAIRequest, model models.Model) (*OpenAIResponse, error)
	Completion(ctx context.Context, req OpenAIRequest, model models.Model) (*OpenAIResponse, error)
	Models(ctx context.Context, model models.Model) (*OpenAIModelsResponse, error)
}

type OpenAIRequest struct {
	Model       string                 `json:"model"`
	Messages    []ChatMessage          `json:"messages,omitempty"`
	Prompt      string                 `json:"prompt,omitempty"`
	Stream      bool                   `json:"stream,omitempty"`
	Temperature float64                `json:"temperature,omitempty"`
	MaxTokens   int                    `json:"max_tokens,omitempty"`
	TopP        float64                `json:"top_p,omitempty"`
	N           int                    `json:"n,omitempty"`
	Stop        []string               `json:"stop,omitempty"`
	RawBody     map[string]interface{} `json:"-"`
	StreamFunc  func(string)           `json:"-"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

type OpenAIResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message,omitempty"`
	Text         string      `json:"text,omitempty"`
	FinishReason string      `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

type OpenAIModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

type Model struct {
	ID          string   `json:"id"`
	Object      string   `json:"object"`
	Created     int64    `json:"created"`
	OwnedBy     string   `json:"owned_by"`
	Permissions []string `json:"permissions,omitempty"`
}

type APIError struct {
	Message    string      `json:"message"`
	Type       string      `json:"type,omitempty"`
	Param      interface{} `json:"param,omitempty"`
	Code       interface{} `json:"code,omitempty"`
	HTTPStatus int         `json:"-"`
}

func (e *APIError) Error() string {
	return e.Message
}

type ResilienceOptions struct {
	RetryAttempts       int
	RetryBackoff        time.Duration
	FailureThreshold    int
	OpenTimeout         time.Duration
	HalfOpenMaxRequests int
}

type HTTPClient struct {
	Client     *http.Client
	Timeout    time.Duration
	resilience ResilienceOptions
	breakers   map[string]*circuitBreaker
	mu         sync.Mutex
}

func NewHTTPClient(timeout time.Duration, resilience ResilienceOptions) *HTTPClient {
	if resilience.RetryAttempts < 0 {
		resilience.RetryAttempts = 0
	}
	if resilience.RetryBackoff <= 0 {
		resilience.RetryBackoff = 200 * time.Millisecond
	}
	if resilience.FailureThreshold <= 0 {
		resilience.FailureThreshold = 5
	}
	if resilience.OpenTimeout <= 0 {
		resilience.OpenTimeout = 30 * time.Second
	}
	if resilience.HalfOpenMaxRequests <= 0 {
		resilience.HalfOpenMaxRequests = 1
	}

	return &HTTPClient{
		Client:     &http.Client{Timeout: timeout},
		Timeout:    timeout,
		resilience: resilience,
		breakers:   make(map[string]*circuitBreaker),
	}
}

func (c *HTTPClient) Post(ctx context.Context, url string, body interface{}, headers map[string]string) (*http.Response, error) {
	return c.PostWithFailover(ctx, url, nil, body, headers)
}

func (c *HTTPClient) PostWithFailover(ctx context.Context, url string, fallbackURLs []string, body interface{}, headers map[string]string) (*http.Response, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	reqHeaders := cloneHeaders(headers)
	if _, exists := reqHeaders["Content-Type"]; !exists {
		reqHeaders["Content-Type"] = "application/json"
	}

	return c.doWithResilience(ctx, http.MethodPost, url, fallbackURLs, jsonBody, reqHeaders)
}

func (c *HTTPClient) Get(ctx context.Context, url string, headers map[string]string) (*http.Response, error) {
	return c.GetWithFailover(ctx, url, nil, headers)
}

func (c *HTTPClient) GetWithFailover(ctx context.Context, url string, fallbackURLs []string, headers map[string]string) (*http.Response, error) {
	return c.doWithResilience(ctx, http.MethodGet, url, fallbackURLs, nil, cloneHeaders(headers))
}

func (c *HTTPClient) doWithResilience(ctx context.Context, method string, url string, fallbackURLs []string, payload []byte, headers map[string]string) (*http.Response, error) {
	candidates := buildURLCandidates(url, fallbackURLs)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no upstream url configured")
	}

	var lastErr error
	skippedByCircuit := 0

	for _, candidate := range candidates {
		breaker := c.getBreaker(candidate)
		if !breaker.Allow(c.resilience.OpenTimeout, c.resilience.HalfOpenMaxRequests) {
			skippedByCircuit++
			continue
		}

		attempts := c.resilience.RetryAttempts + 1
		var endpointErr error

		for attempt := 0; attempt < attempts; attempt++ {
			var requestBody io.Reader
			if payload != nil {
				requestBody = bytes.NewReader(payload)
			}

			req, err := http.NewRequestWithContext(ctx, method, candidate, requestBody)
			if err != nil {
				endpointErr = fmt.Errorf("failed to create request: %w", err)
				break
			}
			for k, v := range headers {
				req.Header.Set(k, v)
			}

			resp, err := c.Client.Do(req)
			if err != nil {
				endpointErr = fmt.Errorf("request failed: %w", err)
				if attempt < attempts-1 {
					if waitErr := waitForRetry(ctx, c.retryBackoff(attempt)); waitErr != nil {
						return nil, waitErr
					}
					continue
				}
				break
			}

			if isRetryableStatus(resp.StatusCode) {
				endpointErr = fmt.Errorf("upstream status %d", resp.StatusCode)
				drainAndClose(resp)
				if attempt < attempts-1 {
					if waitErr := waitForRetry(ctx, c.retryBackoff(attempt)); waitErr != nil {
						return nil, waitErr
					}
					continue
				}
				break
			}

			breaker.OnSuccess()
			return resp, nil
		}

		breaker.OnFailure(c.resilience.FailureThreshold)
		if endpointErr != nil {
			lastErr = fmt.Errorf("upstream %s failed: %w", candidate, endpointErr)
		} else {
			lastErr = fmt.Errorf("upstream %s failed", candidate)
		}
	}

	if skippedByCircuit == len(candidates) {
		return nil, fmt.Errorf("all upstream circuits are open")
	}
	if lastErr != nil {
		return nil, lastErr
	}

	return nil, fmt.Errorf("all upstreams unavailable")
}

func (c *HTTPClient) retryBackoff(attempt int) time.Duration {
	backoff := c.resilience.RetryBackoff
	for i := 0; i < attempt; i++ {
		backoff *= 2
	}
	return backoff
}

func (c *HTTPClient) getBreaker(endpoint string) *circuitBreaker {
	c.mu.Lock()
	defer c.mu.Unlock()

	if breaker, exists := c.breakers[endpoint]; exists {
		return breaker
	}

	breaker := &circuitBreaker{state: circuitClosed}
	c.breakers[endpoint] = breaker
	return breaker
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return map[string]string{}
	}

	cloned := make(map[string]string, len(headers))
	for k, v := range headers {
		cloned[k] = v
	}
	return cloned
}

func buildURLCandidates(primary string, fallbacks []string) []string {
	seen := make(map[string]struct{}, 1+len(fallbacks))
	candidates := make([]string, 0, 1+len(fallbacks))

	add := func(raw string) {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return
		}
		if _, exists := seen[trimmed]; exists {
			return
		}
		seen[trimmed] = struct{}{}
		candidates = append(candidates, trimmed)
	}

	add(primary)
	for _, fallback := range fallbacks {
		add(fallback)
	}

	return candidates
}

func BuildEndpoint(baseURL, path string) string {
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")
}

func BuildFallbackEndpoints(baseURLs []string, path string) []string {
	endpoints := make([]string, 0, len(baseURLs))
	for _, baseURL := range baseURLs {
		trimmed := strings.TrimSpace(baseURL)
		if trimmed == "" {
			continue
		}
		endpoints = append(endpoints, BuildEndpoint(trimmed, path))
	}
	return endpoints
}

func waitForRetry(ctx context.Context, backoff time.Duration) error {
	timer := time.NewTimer(backoff)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isRetryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

type circuitState int

const (
	circuitClosed circuitState = iota
	circuitOpen
	circuitHalfOpen
)

type circuitBreaker struct {
	mu                  sync.Mutex
	state               circuitState
	consecutiveFailures int
	openedAt            time.Time
	halfOpenRequests    int
}

func (c *circuitBreaker) Allow(openTimeout time.Duration, halfOpenMaxRequests int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.state {
	case circuitClosed:
		return true
	case circuitOpen:
		if time.Since(c.openedAt) < openTimeout {
			return false
		}
		c.state = circuitHalfOpen
		c.halfOpenRequests = 0
	}

	if c.halfOpenRequests >= halfOpenMaxRequests {
		return false
	}
	c.halfOpenRequests++
	return true
}

func (c *circuitBreaker) OnSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.state = circuitClosed
	c.consecutiveFailures = 0
	c.halfOpenRequests = 0
}

func (c *circuitBreaker) OnFailure(failureThreshold int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state == circuitHalfOpen {
		c.state = circuitOpen
		c.openedAt = time.Now()
		c.consecutiveFailures = 1
		c.halfOpenRequests = 0
		return
	}

	c.consecutiveFailures++
	if c.consecutiveFailures < failureThreshold {
		return
	}

	c.state = circuitOpen
	c.openedAt = time.Now()
	c.halfOpenRequests = 0
}

func ParseErrorResponse(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read error response: %w", err)
	}

	var errResp struct {
		Error struct {
			Message string      `json:"message"`
			Type    string      `json:"type"`
			Param   interface{} `json:"param"`
			Code    interface{} `json:"code"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
		return &APIError{
			Message:    errResp.Error.Message,
			Type:       errResp.Error.Type,
			Param:      errResp.Error.Param,
			Code:       errResp.Error.Code,
			HTTPStatus: resp.StatusCode,
		}
	}

	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		trimmed = http.StatusText(resp.StatusCode)
	}

	return &APIError{
		Message:    fmt.Sprintf("upstream HTTP %d: %s", resp.StatusCode, trimmed),
		Type:       "upstream_error",
		Code:       resp.StatusCode,
		HTTPStatus: resp.StatusCode,
	}
}

func ConvertChatMessages(messages []ChatMessage) []map[string]string {
	result := make([]map[string]string, len(messages))
	for i, msg := range messages {
		result[i] = map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		}
		if msg.Name != "" {
			result[i]["name"] = msg.Name
		}
	}
	return result
}

func (r OpenAIRequest) Payload() map[string]interface{} {
	if len(r.RawBody) > 0 {
		payload, ok := cloneJSONValue(r.RawBody).(map[string]interface{})
		if ok {
			if r.Model != "" {
				payload["model"] = r.Model
			}
			return payload
		}
	}

	payload := map[string]interface{}{
		"model": r.Model,
	}
	if len(r.Messages) > 0 {
		payload["messages"] = r.Messages
	}
	if r.Prompt != "" {
		payload["prompt"] = r.Prompt
	}
	if r.Stream {
		payload["stream"] = true
	}
	if r.Temperature >= 0 {
		payload["temperature"] = r.Temperature
	}
	if r.MaxTokens > 0 {
		payload["max_tokens"] = r.MaxTokens
	}
	if r.TopP > 0 {
		payload["top_p"] = r.TopP
	}
	if r.N > 0 {
		payload["n"] = r.N
	}
	if len(r.Stop) > 0 {
		payload["stop"] = r.Stop
	}
	return payload
}

func cloneJSONValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		cloned := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			cloned[key] = cloneJSONValue(item)
		}
		return cloned
	case []interface{}:
		cloned := make([]interface{}, len(typed))
		for i, item := range typed {
			cloned[i] = cloneJSONValue(item)
		}
		return cloned
	default:
		return typed
	}
}
