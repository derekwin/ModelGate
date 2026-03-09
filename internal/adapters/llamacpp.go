package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
		return streamOpenAICompatible(ctx, a.HTTPClient, primaryURL, fallbackURLs, req, headers, "chat")
	}

	resp, err := a.HTTPClient.PostWithFailover(ctx, primaryURL, fallbackURLs, req.Payload(), headers)
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
func (a *LlamaCppAdapter) Completion(ctx context.Context, req OpenAIRequest, model models.Model) (*OpenAIResponse, error) {
	baseURL := model.BaseURL
	if baseURL == "" {
		baseURL = a.BaseURL
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

	if req.Stream {
		return streamOpenAICompatible(ctx, a.HTTPClient, primaryURL, fallbackURLs, req, headers, "completion")
	}

	resp, err := a.HTTPClient.PostWithFailover(ctx, primaryURL, fallbackURLs, req.Payload(), headers)
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
