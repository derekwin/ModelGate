package adapters

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"modelgate/internal/models"
)

type VLLMAdapter struct {
	HTTPClient *HTTPClient
	BaseURL    string
}

func NewVLLMAdapter(baseURL string, timeout int64) *VLLMAdapter {
	return &VLLMAdapter{
		HTTPClient: NewHTTPClient(time.Duration(timeout) * time.Second),
		BaseURL:    baseURL,
	}
}

func (a *VLLMAdapter) ChatCompletion(ctx context.Context, req OpenAIRequest, model models.Model) (*OpenAIResponse, error) {
	baseURL := model.BaseURL
	if baseURL == "" {
		baseURL = a.BaseURL
	}

	vllmReq := map[string]interface{}{
		"model":    req.Model,
		"messages": req.Messages,
	}
	if req.Stream {
		vllmReq["stream"] = true
	}
	if req.Temperature >= 0 {
		vllmReq["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		vllmReq["max_tokens"] = req.MaxTokens
	}
	if req.TopP > 0 {
		vllmReq["top_p"] = req.TopP
	}
	if req.N > 0 {
		vllmReq["n"] = req.N
	}
	if len(req.Stop) > 0 {
		vllmReq["stop"] = req.Stop
	}

	headers := map[string]string{
		"Content-Type": "application/json",
	}

	if model.APIKey != "" {
		headers["Authorization"] = "Bearer " + model.APIKey
	}

	if req.Stream {
		return a.streamChatCompletion(ctx, baseURL, vllmReq, headers, req.StreamFunc)
	}

	resp, err := a.HTTPClient.Post(ctx, baseURL+"/v1/chat/completions", vllmReq, headers)
	if err != nil {
		return nil, fmt.Errorf("vllm request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	var vllmResp OpenAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&vllmResp); err != nil {
		return nil, fmt.Errorf("failed to decode vllm response: %w", err)
	}

	return &vllmResp, nil
}

func (a *VLLMAdapter) streamChatCompletion(ctx context.Context, baseURL string, req map[string]interface{}, headers map[string]string, streamFunc func(string)) (*OpenAIResponse, error) {
	req["stream"] = true
	jsonReq, _ := json.Marshal(req)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/v1/chat/completions", bytes.NewBuffer(jsonReq))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := a.HTTPClient.Client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("vllm stream request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	var fullContent string
	var totalPrompt, totalCompletion int64

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

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int64 `json:"prompt_tokens"`
				CompletionTokens int64 `json:"completion_tokens"`
			} `json:"usage"`
		}

		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}

		if len(chunk.Choices) > 0 {
			fullContent += chunk.Choices[0].Delta.Content
			if streamFunc != nil {
				streamFunc("data: " + payload + "\n\n")
			}
			if chunk.Choices[0].FinishReason != "" {
				break
			}
		}
		totalPrompt = chunk.Usage.PromptTokens
		totalCompletion = chunk.Usage.CompletionTokens
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read stream: %w", err)
	}

	return &OpenAIResponse{
		ID:      fmt.Sprintf("chatcmpl-%s", generateID()),
		Object:  "chat.completion",
		Created: getTimestamp(),
		Model:   req["model"].(string),
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
			PromptTokens:     totalPrompt,
			CompletionTokens: totalCompletion,
			TotalTokens:      totalPrompt + totalCompletion,
		},
	}, nil
}

func (a *VLLMAdapter) Completion(ctx context.Context, req OpenAIRequest, model models.Model) (*OpenAIResponse, error) {
	baseURL := model.BaseURL
	if baseURL == "" {
		baseURL = a.BaseURL
	}

	vllmReq := map[string]interface{}{
		"model":  req.Model,
		"prompt": req.Prompt,
	}
	if req.Stream {
		vllmReq["stream"] = true
	}
	if req.Temperature >= 0 {
		vllmReq["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		vllmReq["max_tokens"] = req.MaxTokens
	}
	if req.TopP > 0 {
		vllmReq["top_p"] = req.TopP
	}
	if req.N > 0 {
		vllmReq["n"] = req.N
	}
	if len(req.Stop) > 0 {
		vllmReq["stop"] = req.Stop
	}

	headers := map[string]string{
		"Content-Type": "application/json",
	}

	if model.APIKey != "" {
		headers["Authorization"] = "Bearer " + model.APIKey
	}

	resp, err := a.HTTPClient.Post(ctx, baseURL+"/v1/completions", vllmReq, headers)
	if err != nil {
		return nil, fmt.Errorf("vllm request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	var vllmResp OpenAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&vllmResp); err != nil {
		return nil, fmt.Errorf("failed to decode vllm response: %w", err)
	}

	return &vllmResp, nil
}

func (a *VLLMAdapter) Models(ctx context.Context, model models.Model) (*OpenAIModelsResponse, error) {
	baseURL := model.BaseURL
	if baseURL == "" {
		baseURL = a.BaseURL
	}

	resp, err := a.HTTPClient.Get(ctx, baseURL+"/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("vllm models request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	var vllmResp OpenAIModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&vllmResp); err != nil {
		return nil, fmt.Errorf("failed to decode vllm models response: %w", err)
	}

	return &vllmResp, nil
}
