package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"modelgate/internal/models"
)

type OpenAIAdapter struct {
	HTTPClient *HTTPClient
	BaseURL    string
	APIKey     string
}

func NewOpenAIAdapter(baseURL, apiKey string, timeout int64) *OpenAIAdapter {
	return &OpenAIAdapter{
		HTTPClient: NewHTTPClient(time.Duration(timeout) * time.Second),
		BaseURL:    baseURL,
		APIKey:     apiKey,
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

	openaiReq := map[string]interface{}{
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
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + apiKey,
	}

	if req.Stream {
		return a.streamChatCompletion(ctx, baseURL, req, headers)
	}

	resp, err := a.HTTPClient.Post(ctx, baseURL+"/chat/completions", openaiReq, headers)
	if err != nil {
		return nil, fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	var openaiResp OpenAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&openaiResp); err != nil {
		return nil, fmt.Errorf("failed to decode openai response: %w", err)
	}

	return &openaiResp, nil
}

func (a *OpenAIAdapter) streamChatCompletion(ctx context.Context, baseURL string, req OpenAIRequest, headers map[string]string) (*OpenAIResponse, error) {
	openaiReq := map[string]interface{}{
		"model":       req.Model,
		"messages":    req.Messages,
		"stream":      true,
		"temperature": req.Temperature,
		"max_tokens":  req.MaxTokens,
		"top_p":       req.TopP,
		"n":           req.N,
		"stop":        req.Stop,
	}

	jsonReq, _ := json.Marshal(openaiReq)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/chat/completions", bytes.NewBuffer(jsonReq))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := a.HTTPClient.Client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai stream request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	dec := json.NewDecoder(resp.Body)
	var fullContent string

	for {
		var chunk map[string]interface{}
		if err := dec.Decode(&chunk); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to decode stream chunk: %w", err)
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
			jsonData, _ := json.Marshal(chunk)
			req.StreamFunc("data: " + string(jsonData) + "\n\n")
		}

		finishReason, ok := choice["finish_reason"].(string)
		if ok && finishReason == "stop" {
			break
		}
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

func (a *OpenAIAdapter) Completion(ctx context.Context, req OpenAIRequest, model models.Model) (*OpenAIResponse, error) {
	baseURL := model.BaseURL
	if baseURL == "" {
		baseURL = a.BaseURL
	}

	apiKey := model.APIKey
	if apiKey == "" {
		apiKey = a.APIKey
	}

	openaiReq := map[string]interface{}{
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
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + apiKey,
	}

	resp, err := a.HTTPClient.Post(ctx, baseURL+"/completions", openaiReq, headers)
	if err != nil {
		return nil, fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	var openaiResp OpenAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&openaiResp); err != nil {
		return nil, fmt.Errorf("failed to decode openai response: %w", err)
	}

	return &openaiResp, nil
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

	resp, err := a.HTTPClient.Get(ctx, baseURL+"/models", headers)
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
