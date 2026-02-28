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
	HTTPClient *HTTPClient
	BaseURL    string
	APIKey     string
}

func NewAPI3Adapter(baseURL, apiKey string, timeout int64) *API3Adapter {
	return &API3Adapter{
		HTTPClient: NewHTTPClient(time.Duration(timeout) * time.Second),
		BaseURL:    baseURL,
		APIKey:     apiKey,
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
		"model":       req.Model,
		"messages":    req.Messages,
		"stream":      req.Stream,
		"temperature": req.Temperature,
		"max_tokens":  req.MaxTokens,
		"top_p":       req.TopP,
		"n":           req.N,
		"stop":        req.Stop,
	}

	headers := map[string]string{
		"Content-Type": "application/json",
		"x-api-key":    apiKey,
	}

	resp, err := a.HTTPClient.Post(ctx, baseURL+"/chat/completions", api3Req, headers)
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
		"model":       req.Model,
		"prompt":      req.Prompt,
		"stream":      req.Stream,
		"temperature": req.Temperature,
		"max_tokens":  req.MaxTokens,
		"top_p":       req.TopP,
		"n":           req.N,
		"stop":        req.Stop,
	}

	headers := map[string]string{
		"Content-Type": "application/json",
		"x-api-key":    apiKey,
	}

	resp, err := a.HTTPClient.Post(ctx, baseURL+"/completions", api3Req, headers)
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

	resp, err := a.HTTPClient.Get(ctx, baseURL+"/models", headers)
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
