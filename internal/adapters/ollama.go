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

	messages, err := extractRawMessages(req)
	if err != nil {
		return nil, err
	}

	ollamaReq := map[string]interface{}{
		"model":    req.Model,
		"messages": convertToOllamaMessages(messages),
		"stream":   req.Stream,
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
		return a.streamChatCompletion(ctx, primaryURL, fallbackURLs, req, ollamaReq)
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
		Done            bool `json:"done"`
		PromptEvalCount int  `json:"prompt_eval_count"`
		EvalCount       int  `json:"eval_count"`
		Usage           struct {
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
			PromptTokens:     int64(resolveOllamaPromptTokens(ollamaResp.Usage.PromptTokens, ollamaResp.PromptEvalCount)),
			CompletionTokens: int64(resolveOllamaCompletionTokens(ollamaResp.Usage.CompletionTokens, ollamaResp.EvalCount)),
			TotalTokens: int64(resolveOllamaTotalTokens(
				ollamaResp.Usage.TotalTokens,
				resolveOllamaPromptTokens(ollamaResp.Usage.PromptTokens, ollamaResp.PromptEvalCount),
				resolveOllamaCompletionTokens(ollamaResp.Usage.CompletionTokens, ollamaResp.EvalCount),
			)),
		},
	}, nil
}

func (a *OllamaAdapter) streamChatCompletion(ctx context.Context, primaryURL string, fallbackURLs []string, req OpenAIRequest, ollamaReq map[string]interface{}) (*OpenAIResponse, error) {
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
	streamID := fmt.Sprintf("chatcmpl-%s", generateID())
	created := getTimestamp()
	sentRole := false

	for {
		var chunk struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			Done            bool `json:"done"`
			PromptEvalCount int  `json:"prompt_eval_count"`
			EvalCount       int  `json:"eval_count"`
			Usage           struct {
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
		totalPrompt = resolveOllamaPromptTokens(chunk.Usage.PromptTokens, chunk.PromptEvalCount)
		totalCompletion = resolveOllamaCompletionTokens(chunk.Usage.CompletionTokens, chunk.EvalCount)

		if req.StreamFunc != nil {
			if !sentRole {
				roleChunk := map[string]interface{}{
					"id":      streamID,
					"object":  "chat.completion.chunk",
					"created": created,
					"model":   req.Model,
					"choices": []map[string]interface{}{
						{
							"index": 0,
							"delta": map[string]string{
								"role": "assistant",
							},
							"finish_reason": nil,
						},
					},
				}
				jsonData, _ := json.Marshal(roleChunk)
				req.StreamFunc("data: " + string(jsonData) + "\n\n")
				sentRole = true
			}

			if chunk.Message.Content != "" {
				data := map[string]interface{}{
					"id":      streamID,
					"object":  "chat.completion.chunk",
					"created": created,
					"model":   req.Model,
					"choices": []map[string]interface{}{
						{
							"index": 0,
							"delta": map[string]string{
								"content": chunk.Message.Content,
							},
							"finish_reason": nil,
						},
					},
				}
				jsonData, _ := json.Marshal(data)
				req.StreamFunc("data: " + string(jsonData) + "\n\n")
			}

			if chunk.Done {
				doneChunk := map[string]interface{}{
					"id":      streamID,
					"object":  "chat.completion.chunk",
					"created": created,
					"model":   req.Model,
					"choices": []map[string]interface{}{
						{
							"index":         0,
							"delta":         map[string]interface{}{},
							"finish_reason": "stop",
						},
					},
					"usage": map[string]int{
						"prompt_tokens":     totalPrompt,
						"completion_tokens": totalCompletion,
						"total_tokens":      totalPrompt + totalCompletion,
					},
				}
				jsonData, _ := json.Marshal(doneChunk)
				req.StreamFunc("data: " + string(jsonData) + "\n\n")
			}
		}

		if chunk.Done {
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

	prompt, err := extractPromptString(req)
	if err != nil {
		return nil, err
	}

	ollamaReq := map[string]interface{}{
		"model":  req.Model,
		"prompt": prompt,
		"stream": req.Stream,
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

	if req.Stream {
		return a.streamCompletion(ctx, primaryURL, fallbackURLs, req, ollamaReq)
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
		Response        string `json:"response"`
		Done            bool   `json:"done"`
		PromptEvalCount int    `json:"prompt_eval_count"`
		EvalCount       int    `json:"eval_count"`
		Usage           struct {
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
		Object:  "text_completion",
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
			PromptTokens:     int64(resolveOllamaPromptTokens(ollamaResp.Usage.PromptTokens, ollamaResp.PromptEvalCount)),
			CompletionTokens: int64(resolveOllamaCompletionTokens(ollamaResp.Usage.CompletionTokens, ollamaResp.EvalCount)),
			TotalTokens: int64(resolveOllamaTotalTokens(
				ollamaResp.Usage.TotalTokens,
				resolveOllamaPromptTokens(ollamaResp.Usage.PromptTokens, ollamaResp.PromptEvalCount),
				resolveOllamaCompletionTokens(ollamaResp.Usage.CompletionTokens, ollamaResp.EvalCount),
			)),
		},
	}, nil
}

func (a *OllamaAdapter) streamCompletion(ctx context.Context, primaryURL string, fallbackURLs []string, req OpenAIRequest, ollamaReq map[string]interface{}) (*OpenAIResponse, error) {
	resp, err := a.HTTPClient.PostWithFailover(ctx, primaryURL, fallbackURLs, ollamaReq, nil)
	if err != nil {
		return nil, fmt.Errorf("ollama stream request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	dec := json.NewDecoder(resp.Body)
	var fullText string
	var totalPrompt, totalCompletion int
	streamID := fmt.Sprintf("cmpl-%s", generateID())
	created := getTimestamp()

	for {
		var chunk struct {
			Response        string `json:"response"`
			Done            bool   `json:"done"`
			PromptEvalCount int    `json:"prompt_eval_count"`
			EvalCount       int    `json:"eval_count"`
			Usage           struct {
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

		fullText += chunk.Response
		totalPrompt = resolveOllamaPromptTokens(chunk.Usage.PromptTokens, chunk.PromptEvalCount)
		totalCompletion = resolveOllamaCompletionTokens(chunk.Usage.CompletionTokens, chunk.EvalCount)

		if req.StreamFunc != nil {
			if chunk.Response != "" {
				data := map[string]interface{}{
					"id":      streamID,
					"object":  "text_completion",
					"created": created,
					"model":   req.Model,
					"choices": []map[string]interface{}{
						{
							"index":         0,
							"text":          chunk.Response,
							"finish_reason": nil,
						},
					},
				}
				jsonData, _ := json.Marshal(data)
				req.StreamFunc("data: " + string(jsonData) + "\n\n")
			}

			if chunk.Done {
				doneChunk := map[string]interface{}{
					"id":      streamID,
					"object":  "text_completion",
					"created": created,
					"model":   req.Model,
					"choices": []map[string]interface{}{
						{
							"index":         0,
							"text":          "",
							"finish_reason": "stop",
						},
					},
					"usage": map[string]int{
						"prompt_tokens":     totalPrompt,
						"completion_tokens": totalCompletion,
						"total_tokens":      totalPrompt + totalCompletion,
					},
				}
				jsonData, _ := json.Marshal(doneChunk)
				req.StreamFunc("data: " + string(jsonData) + "\n\n")
			}
		}

		if chunk.Done {
			break
		}
	}

	return &OpenAIResponse{
		ID:      streamID,
		Object:  "text_completion",
		Created: created,
		Model:   req.Model,
		Choices: []Choice{
			{
				Index:        0,
				Text:         fullText,
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

func convertToOllamaMessages(messages []map[string]interface{}) []map[string]string {
	result := make([]map[string]string, 0, len(messages))
	for _, message := range messages {
		role, _ := message["role"].(string)
		item := map[string]string{
			"role":    role,
			"content": normalizeMessageTextContent(message["content"]),
		}
		if name, ok := message["name"].(string); ok && name != "" {
			item["name"] = name
		}
		result = append(result, item)
	}
	return result
}

func resolveOllamaPromptTokens(usagePromptTokens, promptEvalCount int) int {
	if usagePromptTokens > 0 {
		return usagePromptTokens
	}
	return promptEvalCount
}

func resolveOllamaCompletionTokens(usageCompletionTokens, evalCount int) int {
	if usageCompletionTokens > 0 {
		return usageCompletionTokens
	}
	return evalCount
}

func resolveOllamaTotalTokens(usageTotalTokens, promptTokens, completionTokens int) int {
	if usageTotalTokens > 0 {
		return usageTotalTokens
	}
	return promptTokens + completionTokens
}
