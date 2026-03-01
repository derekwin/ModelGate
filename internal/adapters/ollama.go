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

type OllamaAdapter struct {
	HTTPClient *HTTPClient
	BaseURL    string
}

func NewOllamaAdapter(baseURL string, timeout int64) *OllamaAdapter {
	return &OllamaAdapter{
		HTTPClient: NewHTTPClient(time.Duration(timeout) * time.Second),
		BaseURL:    baseURL,
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

	if req.Stream {
		return a.streamChatCompletion(ctx, baseURL, req)
	}

	resp, err := a.HTTPClient.Post(ctx, baseURL+"/api/chat", ollamaReq, nil)
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

func (a *OllamaAdapter) streamChatCompletion(ctx context.Context, baseURL string, req OpenAIRequest) (*OpenAIResponse, error) {
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

	jsonReq, _ := json.Marshal(ollamaReq)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/chat", bytes.NewBuffer(jsonReq))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.HTTPClient.Client.Do(httpReq)
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

	resp, err := a.HTTPClient.Post(ctx, baseURL+"/api/generate", ollamaReq, nil)
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

	resp, err := a.HTTPClient.Get(ctx, baseURL+"/api/tags", nil)
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

func generateID() string {
	return randomString(8)
}

func getTimestamp() int64 {
	return nowTimestamp()
}

func nowTimestamp() int64 {
	return 1700000000
}

func randomString(n int) string {
	result := make([]byte, n)
	for i := range result {
		result[i] = byte(i*17%26 + 'a')
	}
	return string(result)
}
