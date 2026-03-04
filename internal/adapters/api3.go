package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"modelgate/internal/models"
)

type API3Adapter struct {
	HTTPClient   *HTTPClient
	BaseURL      string
	APIKey       string
	FallbackURLs []string
}

func NewAPI3Adapter(baseURL, apiKey string, fallbackURLs []string, timeoutDuration time.Duration, resilience ResilienceOptions) *API3Adapter {
	return &API3Adapter{
		HTTPClient:   NewHTTPClient(timeoutDuration, resilience),
		BaseURL:      baseURL,
		APIKey:       apiKey,
		FallbackURLs: fallbackURLs,
	}
}

func (a *API3Adapter) ChatCompletion(ctx context.Context, req OpenAIRequest, model models.Model) (*OpenAIResponse, error) {
	baseURL := model.BaseURL
	if baseURL == "" {
		baseURL = a.BaseURL
	}

	apiKey := model.APIKey
	if apiKey == "" {
		apiKey = a.APIKey
	}

	api3Req := map[string]interface{}{
		"model":    req.Model,
		"messages": req.Messages,
	}
	if req.Stream {
		api3Req["stream"] = true
	}
	if req.Temperature >= 0 {
		api3Req["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		api3Req["max_tokens"] = req.MaxTokens
	}
	if req.TopP > 0 {
		api3Req["top_p"] = req.TopP
	}
	if req.N > 0 {
		api3Req["n"] = req.N
	}
	if len(req.Stop) > 0 {
		api3Req["stop"] = req.Stop
	}

	headers := map[string]string{
		"Content-Type": "application/json",
		"x-api-key":    apiKey,
	}

	chatPath := "/chat/completions"
	primaryURL := BuildEndpoint(baseURL, chatPath)
	fallbackURLs := BuildFallbackEndpoints(a.FallbackURLs, chatPath)

	resp, err := a.HTTPClient.PostWithFailover(ctx, primaryURL, fallbackURLs, api3Req, headers)
	if err != nil {
		return nil, fmt.Errorf("api3 request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	var api3Resp OpenAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&api3Resp); err != nil {
		return nil, fmt.Errorf("failed to decode api3 response: %w", err)
	}

	return &api3Resp, nil
}

func (a *API3Adapter) Completion(ctx context.Context, req OpenAIRequest, model models.Model) (*OpenAIResponse, error) {
	baseURL := model.BaseURL
	if baseURL == "" {
		baseURL = a.BaseURL
	}

	apiKey := model.APIKey
	if apiKey == "" {
		apiKey = a.APIKey
	}

	api3Req := map[string]interface{}{
		"model":  req.Model,
		"prompt": req.Prompt,
	}
	if req.Stream {
		api3Req["stream"] = true
	}
	if req.Temperature >= 0 {
		api3Req["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		api3Req["max_tokens"] = req.MaxTokens
	}
	if req.TopP > 0 {
		api3Req["top_p"] = req.TopP
	}
	if req.N > 0 {
		api3Req["n"] = req.N
	}
	if len(req.Stop) > 0 {
		api3Req["stop"] = req.Stop
	}

	headers := map[string]string{
		"Content-Type": "application/json",
		"x-api-key":    apiKey,
	}

	completionPath := "/completions"
	primaryURL := BuildEndpoint(baseURL, completionPath)
	fallbackURLs := BuildFallbackEndpoints(a.FallbackURLs, completionPath)

	resp, err := a.HTTPClient.PostWithFailover(ctx, primaryURL, fallbackURLs, api3Req, headers)
	if err != nil {
		return nil, fmt.Errorf("api3 request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	var api3Resp OpenAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&api3Resp); err != nil {
		return nil, fmt.Errorf("failed to decode api3 response: %w", err)
	}

	return &api3Resp, nil
}

func (a *API3Adapter) Models(ctx context.Context, model models.Model) (*OpenAIModelsResponse, error) {
	baseURL := model.BaseURL
	if baseURL == "" {
		baseURL = a.BaseURL
	}

	apiKey := model.APIKey
	if apiKey == "" {
		apiKey = a.APIKey
	}

	headers := map[string]string{
		"x-api-key": apiKey,
	}

	modelsPath := "/models"
	primaryURL := BuildEndpoint(baseURL, modelsPath)
	fallbackURLs := BuildFallbackEndpoints(a.FallbackURLs, modelsPath)

	resp, err := a.HTTPClient.GetWithFailover(ctx, primaryURL, fallbackURLs, headers)
	if err != nil {
		return nil, fmt.Errorf("api3 models request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	var api3Resp OpenAIModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&api3Resp); err != nil {
		return nil, fmt.Errorf("failed to decode api3 models response: %w", err)
	}

	return &api3Resp, nil
}
