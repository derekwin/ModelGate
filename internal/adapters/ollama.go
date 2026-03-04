package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"modelgate/internal/models"
)

type OllamaAdapter struct {
	HTTPClient   *HTTPClient
	BaseURL      string
	FallbackURLs []string
}

func NewOllamaAdapter(baseURL string, fallbackURLs []string, timeoutDuration time.Duration, resilience ResilienceOptions) *OllamaAdapter {
	return &OllamaAdapter{
		HTTPClient:   NewHTTPClient(timeoutDuration, resilience),
		BaseURL:      baseURL,
		FallbackURLs: fallbackURLs,
	}
}

func (a *OllamaAdapter) ChatCompletion(ctx context.Context, req OpenAIRequest, model models.Model) (*OpenAIResponse, error) {
	baseURL := model.BaseURL
	if baseURL == "" {
		baseURL = a.BaseURL
	}

	ollamaReq := map[string]interface{}{
		"model":    req.Model,
		"messages": ConvertChatMessages(req.Messages),
	}
	if req.Stream {
		ollamaReq["stream"] = true
	}
	if req.Temperature >= 0 {
		ollamaReq["temperature"] = req.Temperature
	}
	if req.TopP > 0 {
		ollamaReq["top_p"] = req.TopP
	}
	if req.MaxTokens > 0 {
		ollamaReq["num_predict"] = req.MaxTokens
	}
	if len(req.Stop) > 0 {
		ollamaReq["stop"] = req.Stop
	}

	chatPath := "/api/chat"
	primaryURL := BuildEndpoint(baseURL, chatPath)
	fallbackURLs := BuildFallbackEndpoints(a.FallbackURLs, chatPath)

	if req.Stream {
		return a.streamChatCompletion(ctx, primaryURL, fallbackURLs, req)
	}

	resp, err := a.HTTPClient.PostWithFailover(ctx, primaryURL, fallbackURLs, ollamaReq, nil)
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	var ollamaResp struct {
		Message struct {
			Content string `json:"content"`
			Role    string `json:"role"`
		} `json:"message"`
		Done  bool `json:"done"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to decode ollama response: %w", err)
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
					Role:    ollamaResp.Message.Role,
					Content: ollamaResp.Message.Content,
				},
				FinishReason: "stop",
			},
		},
		Usage: Usage{
			PromptTokens:     int64(ollamaResp.Usage.PromptTokens),
			CompletionTokens: int64(ollamaResp.Usage.CompletionTokens),
			TotalTokens:      int64(ollamaResp.Usage.TotalTokens),
		},
	}, nil
}

func (a *OllamaAdapter) streamChatCompletion(ctx context.Context, primaryURL string, fallbackURLs []string, req OpenAIRequest) (*OpenAIResponse, error) {
	ollamaReq := map[string]interface{}{
		"model":    req.Model,
		"messages": ConvertChatMessages(req.Messages),
		"stream":   true,
	}
	if req.Temperature >= 0 {
		ollamaReq["temperature"] = req.Temperature
	}
	if req.TopP > 0 {
		ollamaReq["top_p"] = req.TopP
	}
	if req.MaxTokens > 0 {
		ollamaReq["num_predict"] = req.MaxTokens
	}
	if len(req.Stop) > 0 {
		ollamaReq["stop"] = req.Stop
	}

	resp, err := a.HTTPClient.PostWithFailover(ctx, primaryURL, fallbackURLs, ollamaReq, nil)
	if err != nil {
		return nil, fmt.Errorf("ollama stream request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	dec := json.NewDecoder(resp.Body)
	var fullContent string
	var totalPrompt, totalCompletion int

	for {
		var chunk struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Done  bool `json:"done"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}

		if err := dec.Decode(&chunk); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to decode stream chunk: %w", err)
		}

		fullContent += chunk.Message.Content
		totalPrompt = chunk.Usage.PromptTokens
		totalCompletion = chunk.Usage.CompletionTokens

		if chunk.Done {
			break
		}

		if req.StreamFunc != nil {
			data := map[string]interface{}{
				"choices": []map[string]interface{}{
					{
						"delta": map[string]string{
							"content": chunk.Message.Content,
						},
					},
				},
			}
			jsonData, _ := json.Marshal(data)
			req.StreamFunc("data: " + string(jsonData) + "\n\n")
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
			PromptTokens:     int64(totalPrompt),
			CompletionTokens: int64(totalCompletion),
			TotalTokens:      int64(totalPrompt + totalCompletion),
		},
	}, nil
}

func (a *OllamaAdapter) Completion(ctx context.Context, req OpenAIRequest, model models.Model) (*OpenAIResponse, error) {
	baseURL := model.BaseURL
	if baseURL == "" {
		baseURL = a.BaseURL
	}

	ollamaReq := map[string]interface{}{
		"model":  req.Model,
		"prompt": req.Prompt,
	}
	if req.Stream {
		ollamaReq["stream"] = true
	}
	if req.Temperature >= 0 {
		ollamaReq["temperature"] = req.Temperature
	}
	if req.TopP > 0 {
		ollamaReq["top_p"] = req.TopP
	}
	if req.N > 0 {
		ollamaReq["n"] = req.N
	}
	if len(req.Stop) > 0 {
		ollamaReq["stop"] = req.Stop
	}
	if req.MaxTokens > 0 {
		ollamaReq["num_predict"] = req.MaxTokens
	}

	completionPath := "/api/generate"
	primaryURL := BuildEndpoint(baseURL, completionPath)
	fallbackURLs := BuildFallbackEndpoints(a.FallbackURLs, completionPath)

	resp, err := a.HTTPClient.PostWithFailover(ctx, primaryURL, fallbackURLs, ollamaReq, nil)
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	var ollamaResp struct {
		Response string `json:"response"`
		Done     bool   `json:"done"`
		Usage    struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to decode ollama response: %w", err)
	}

	return &OpenAIResponse{
		ID:      fmt.Sprintf("cmpl-%s", generateID()),
		Object:  "text.completion",
		Created: getTimestamp(),
		Model:   req.Model,
		Choices: []Choice{
			{
				Index:        0,
				Text:         ollamaResp.Response,
				FinishReason: "stop",
			},
		},
		Usage: Usage{
			PromptTokens:     int64(ollamaResp.Usage.PromptTokens),
			CompletionTokens: int64(ollamaResp.Usage.CompletionTokens),
			TotalTokens:      int64(ollamaResp.Usage.TotalTokens),
		},
	}, nil
}

func (a *OllamaAdapter) Models(ctx context.Context, model models.Model) (*OpenAIModelsResponse, error) {
	baseURL := model.BaseURL
	if baseURL == "" {
		baseURL = a.BaseURL
	}

	modelsPath := "/api/tags"
	primaryURL := BuildEndpoint(baseURL, modelsPath)
	fallbackURLs := BuildFallbackEndpoints(a.FallbackURLs, modelsPath)

	resp, err := a.HTTPClient.GetWithFailover(ctx, primaryURL, fallbackURLs, nil)
	if err != nil {
		return nil, fmt.Errorf("ollama models request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	var ollamaResp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to decode ollama models response: %w", err)
	}

	models := make([]Model, len(ollamaResp.Models))
	for i, m := range ollamaResp.Models {
		models[i] = Model{
			ID:      m.Name,
			Object:  "model",
			Created: getTimestamp(),
			OwnedBy: "ollama",
		}
	}

	return &OpenAIModelsResponse{
		Object: "list",
		Data:   models,
	}, nil
}
