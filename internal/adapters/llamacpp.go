package adapters

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"modelgate/internal/models"
)

type LlamaCppAdapter struct {
	HTTPClient   *HTTPClient
	BaseURL      string
	FallbackURLs []string
}

func NewLlamaCppAdapter(baseURL string, fallbackURLs []string, timeoutDuration time.Duration, resilience ResilienceOptions) *LlamaCppAdapter {
	return &LlamaCppAdapter{
		HTTPClient:   NewHTTPClient(timeoutDuration, resilience),
		BaseURL:      baseURL,
		FallbackURLs: fallbackURLs,
	}
}

func (a *LlamaCppAdapter) ChatCompletion(ctx context.Context, req OpenAIRequest, model models.Model) (*OpenAIResponse, error) {
	baseURL := model.BaseURL
	if baseURL == "" {
		baseURL = a.BaseURL
	}

	llamaReq := map[string]interface{}{
		"model":    req.Model,
		"messages": req.Messages,
	}
	if req.Stream {
		llamaReq["stream"] = true
	}
	if req.Temperature >= 0 {
		llamaReq["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		llamaReq["max_tokens"] = req.MaxTokens
	}
	if req.TopP > 0 {
		llamaReq["top_p"] = req.TopP
	}
	if len(req.Stop) > 0 {
		llamaReq["stop"] = req.Stop
	}

	headers := map[string]string{
		"Content-Type": "application/json",
	}
	if model.APIKey != "" {
		headers["Authorization"] = "Bearer " + model.APIKey
	}

	chatPath := "/v1/chat/completions"
	primaryURL := BuildEndpoint(baseURL, chatPath)
	fallbackURLs := BuildFallbackEndpoints(a.FallbackURLs, chatPath)

	if req.Stream {
		return a.streamChatCompletion(ctx, primaryURL, fallbackURLs, req, headers)
	}

	resp, err := a.HTTPClient.PostWithFailover(ctx, primaryURL, fallbackURLs, llamaReq, headers)
	if err != nil {
		return nil, fmt.Errorf("llama.cpp request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	var llamaResp OpenAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&llamaResp); err != nil {
		return nil, fmt.Errorf("failed to decode llama.cpp response: %w", err)
	}

	return &llamaResp, nil
}

func (a *LlamaCppAdapter) streamChatCompletion(ctx context.Context, primaryURL string, fallbackURLs []string, req OpenAIRequest, headers map[string]string) (*OpenAIResponse, error) {
	llamaReq := map[string]interface{}{
		"model":    req.Model,
		"messages": req.Messages,
		"stream":   true,
	}
	if req.Temperature >= 0 {
		llamaReq["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		llamaReq["max_tokens"] = req.MaxTokens
	}
	if req.TopP > 0 {
		llamaReq["top_p"] = req.TopP
	}
	if len(req.Stop) > 0 {
		llamaReq["stop"] = req.Stop
	}

	resp, err := a.HTTPClient.PostWithFailover(ctx, primaryURL, fallbackURLs, llamaReq, headers)
	if err != nil {
		return nil, fmt.Errorf("llama.cpp stream request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	var fullContent string

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}

		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}

		choices, ok := chunk["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			continue
		}

		choice, ok := choices[0].(map[string]interface{})
		if !ok {
			continue
		}

		delta, ok := choice["delta"].(map[string]interface{})
		if !ok {
			continue
		}

		content, ok := delta["content"].(string)
		if ok {
			fullContent += content
		}

		if req.StreamFunc != nil {
			req.StreamFunc("data: " + payload + "\n\n")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read stream: %w", err)
	}

	return &OpenAIResponse{
		ID:      fmt.Sprintf("chatcmpl-%s", generateID()),
		Object:  "chat.completion",
		Created: getTimestamp(),
		Model:   req.Model,
		Choices: []Choice{
			{
				Index: 0,
				Message: ChatMessage{
					Role:    "assistant",
					Content: fullContent,
				},
				FinishReason: "stop",
			},
		},
		Usage: Usage{
			PromptTokens:     int64(len(fullContent) / 4),
			CompletionTokens: int64(len(fullContent) / 4),
			TotalTokens:      int64(len(fullContent) / 2),
		},
	}, nil
}

func (a *LlamaCppAdapter) Completion(ctx context.Context, req OpenAIRequest, model models.Model) (*OpenAIResponse, error) {
	baseURL := model.BaseURL
	if baseURL == "" {
		baseURL = a.BaseURL
	}

	llamaReq := map[string]interface{}{
		"model":  req.Model,
		"prompt": req.Prompt,
	}
	if req.Stream {
		llamaReq["stream"] = true
	}
	if req.Temperature >= 0 {
		llamaReq["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		llamaReq["max_tokens"] = req.MaxTokens
	}
	if req.TopP > 0 {
		llamaReq["top_p"] = req.TopP
	}
	if req.N > 0 {
		llamaReq["n"] = req.N
	}
	if len(req.Stop) > 0 {
		llamaReq["stop"] = req.Stop
	}

	headers := map[string]string{
		"Content-Type": "application/json",
	}
	if model.APIKey != "" {
		headers["Authorization"] = "Bearer " + model.APIKey
	}

	completionPath := "/v1/completions"
	primaryURL := BuildEndpoint(baseURL, completionPath)
	fallbackURLs := BuildFallbackEndpoints(a.FallbackURLs, completionPath)

	resp, err := a.HTTPClient.PostWithFailover(ctx, primaryURL, fallbackURLs, llamaReq, headers)
	if err != nil {
		return nil, fmt.Errorf("llama.cpp request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	var llamaResp OpenAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&llamaResp); err != nil {
		return nil, fmt.Errorf("failed to decode llama.cpp response: %w", err)
	}

	return &llamaResp, nil
}

func (a *LlamaCppAdapter) Models(ctx context.Context, model models.Model) (*OpenAIModelsResponse, error) {
	baseURL := model.BaseURL
	if baseURL == "" {
		baseURL = a.BaseURL
	}

	modelsPath := "/v1/models"
	primaryURL := BuildEndpoint(baseURL, modelsPath)
	fallbackURLs := BuildFallbackEndpoints(a.FallbackURLs, modelsPath)

	resp, err := a.HTTPClient.GetWithFailover(ctx, primaryURL, fallbackURLs, nil)
	if err != nil {
		return nil, fmt.Errorf("llama.cpp models request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	var llamaResp OpenAIModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&llamaResp); err != nil {
		return nil, fmt.Errorf("failed to decode llama.cpp models response: %w", err)
	}

	return &llamaResp, nil
}
