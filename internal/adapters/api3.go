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

	headers := map[string]string{
		"Content-Type": "application/json",
		"x-api-key":    apiKey,
	}

	chatPath := "/chat/completions"
	primaryURL := BuildEndpoint(baseURL, chatPath)
	fallbackURLs := BuildFallbackEndpoints(a.FallbackURLs, chatPath)

	if req.Stream {
		return streamOpenAICompatible(ctx, a.HTTPClient, primaryURL, fallbackURLs, req, headers, "chat")
	}

	resp, err := a.HTTPClient.PostWithFailover(ctx, primaryURL, fallbackURLs, req.Payload(), headers)
	if err != nil {
		return nil, fmt.Errorf("api3 request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	api3Resp, err := decodePreservedResponse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to decode api3 response: %w", err)
	}

	return api3Resp, nil
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

	headers := map[string]string{
		"Content-Type": "application/json",
		"x-api-key":    apiKey,
	}

	completionPath := "/completions"
	primaryURL := BuildEndpoint(baseURL, completionPath)
	fallbackURLs := BuildFallbackEndpoints(a.FallbackURLs, completionPath)

	if req.Stream {
		return streamOpenAICompatible(ctx, a.HTTPClient, primaryURL, fallbackURLs, req, headers, "completion")
	}

	resp, err := a.HTTPClient.PostWithFailover(ctx, primaryURL, fallbackURLs, req.Payload(), headers)
	if err != nil {
		return nil, fmt.Errorf("api3 request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	api3Resp, err := decodePreservedResponse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to decode api3 response: %w", err)
	}

	return api3Resp, nil
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
