package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"modelgate/internal/models"
)

type OpenAIAdapter struct {
	HTTPClient   *HTTPClient
	BaseURL      string
	APIKey       string
	FallbackURLs []string
}

func NewOpenAIAdapter(baseURL, apiKey string, fallbackURLs []string, timeoutDuration time.Duration, resilience ResilienceOptions) *OpenAIAdapter {
	return &OpenAIAdapter{
		HTTPClient:   NewHTTPClient(timeoutDuration, resilience),
		BaseURL:      baseURL,
		APIKey:       apiKey,
		FallbackURLs: fallbackURLs,
	}
}

func (a *OpenAIAdapter) ChatCompletion(ctx context.Context, req OpenAIRequest, model models.Model) (*OpenAIResponse, error) {
	baseURL := model.BaseURL
	if baseURL == "" {
		baseURL = a.BaseURL
	}

	apiKey := model.APIKey
	if apiKey == "" {
		apiKey = a.APIKey
	}

	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + apiKey,
	}

	chatPath := "/chat/completions"
	primaryURL := BuildEndpoint(baseURL, chatPath)
	fallbackURLs := BuildFallbackEndpoints(a.FallbackURLs, chatPath)

	if req.Stream {
		return streamOpenAICompatible(ctx, a.HTTPClient, primaryURL, fallbackURLs, req, headers, "chat")
	}

	resp, err := a.HTTPClient.PostWithFailover(ctx, primaryURL, fallbackURLs, req.Payload(), headers)
	if err != nil {
		return nil, fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	openaiResp, err := decodePreservedResponse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to decode openai response: %w", err)
	}

	return openaiResp, nil
}
func (a *OpenAIAdapter) Completion(ctx context.Context, req OpenAIRequest, model models.Model) (*OpenAIResponse, error) {
	baseURL := model.BaseURL
	if baseURL == "" {
		baseURL = a.BaseURL
	}

	apiKey := model.APIKey
	if apiKey == "" {
		apiKey = a.APIKey
	}

	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + apiKey,
	}

	completionPath := "/completions"
	primaryURL := BuildEndpoint(baseURL, completionPath)
	fallbackURLs := BuildFallbackEndpoints(a.FallbackURLs, completionPath)

	if req.Stream {
		return streamOpenAICompatible(ctx, a.HTTPClient, primaryURL, fallbackURLs, req, headers, "completion")
	}

	resp, err := a.HTTPClient.PostWithFailover(ctx, primaryURL, fallbackURLs, req.Payload(), headers)
	if err != nil {
		return nil, fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	openaiResp, err := decodePreservedResponse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to decode openai response: %w", err)
	}

	return openaiResp, nil
}

func (a *OpenAIAdapter) Models(ctx context.Context, model models.Model) (*OpenAIModelsResponse, error) {
	baseURL := model.BaseURL
	if baseURL == "" {
		baseURL = a.BaseURL
	}

	apiKey := model.APIKey
	if apiKey == "" {
		apiKey = a.APIKey
	}

	headers := map[string]string{
		"Authorization": "Bearer " + apiKey,
	}

	modelsPath := "/models"
	primaryURL := BuildEndpoint(baseURL, modelsPath)
	fallbackURLs := BuildFallbackEndpoints(a.FallbackURLs, modelsPath)

	resp, err := a.HTTPClient.GetWithFailover(ctx, primaryURL, fallbackURLs, headers)
	if err != nil {
		return nil, fmt.Errorf("openai models request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	var openaiResp OpenAIModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&openaiResp); err != nil {
		return nil, fmt.Errorf("failed to decode openai models response: %w", err)
	}

	return &openaiResp, nil
}
