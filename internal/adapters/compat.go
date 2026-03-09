package adapters

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func streamOpenAICompatible(
	ctx context.Context,
	client *HTTPClient,
	primaryURL string,
	fallbackURLs []string,
	req OpenAIRequest,
	headers map[string]string,
	mode string,
) (*OpenAIResponse, error) {
	payload := req.Payload()
	payload["stream"] = true

	resp, err := client.PostWithFailover(ctx, primaryURL, fallbackURLs, payload, headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ParseErrorResponse(resp)
	}

	acc := newStreamAccumulator(mode, req.Model)
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") || !strings.HasPrefix(line, "data:") {
			continue
		}

		event := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if event == "[DONE]" {
			break
		}

		acc.consume(event)
		if req.StreamFunc != nil {
			req.StreamFunc("data: " + event + "\n\n")
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read stream: %w", err)
	}

	return acc.response(), nil
}

type streamAccumulator struct {
	mode       string
	id         string
	object     string
	model      string
	created    int64
	text       string
	finish     string
	usage      Usage
	usageSeen  bool
	chatStream bool
}

func newStreamAccumulator(mode, model string) *streamAccumulator {
	return &streamAccumulator{
		mode:  mode,
		model: model,
	}
}

func (a *streamAccumulator) consume(payload string) {
	var chunk map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return
	}

	if id, ok := chunk["id"].(string); ok && id != "" {
		a.id = id
	}
	if object, ok := chunk["object"].(string); ok && object != "" {
		a.object = object
	}
	if model, ok := chunk["model"].(string); ok && model != "" {
		a.model = model
	}
	if created, ok := jsonInt64(chunk["created"]); ok {
		a.created = created
	}

	if usage, ok := chunk["usage"].(map[string]interface{}); ok {
		a.usage.PromptTokens, _ = jsonInt64(usage["prompt_tokens"])
		a.usage.CompletionTokens, _ = jsonInt64(usage["completion_tokens"])
		a.usage.TotalTokens, _ = jsonInt64(usage["total_tokens"])
		a.usageSeen = true
	}

	choices, ok := chunk["choices"].([]interface{})
	if !ok {
		return
	}

	for _, rawChoice := range choices {
		choice, ok := rawChoice.(map[string]interface{})
		if !ok {
			continue
		}

		if finishReason, ok := choice["finish_reason"].(string); ok && finishReason != "" {
			a.finish = finishReason
		}

		if text, ok := choice["text"].(string); ok {
			a.text += text
		}

		if message, ok := choice["message"].(map[string]interface{}); ok {
			if content, ok := message["content"].(string); ok {
				a.text += content
			}
			a.chatStream = true
		}

		delta, ok := choice["delta"].(map[string]interface{})
		if !ok {
			continue
		}

		a.chatStream = true
		if content, ok := delta["content"].(string); ok {
			a.text += content
		}
	}
}

func (a *streamAccumulator) response() *OpenAIResponse {
	if a.id == "" {
		if a.mode == "completion" {
			a.id = fmt.Sprintf("cmpl-%s", generateID())
		} else {
			a.id = fmt.Sprintf("chatcmpl-%s", generateID())
		}
	}
	if a.object == "" {
		if a.mode == "completion" {
			a.object = "text_completion"
		} else {
			a.object = "chat.completion"
		}
	}
	if a.created == 0 {
		a.created = getTimestamp()
	}
	if a.finish == "" {
		a.finish = "stop"
	}
	if !a.usageSeen {
		a.usage = Usage{}
	}

	response := &OpenAIResponse{
		ID:      a.id,
		Object:  a.object,
		Created: a.created,
		Model:   a.model,
		Usage:   a.usage,
	}

	if a.mode == "completion" && !a.chatStream {
		response.Choices = []Choice{{
			Index:        0,
			Text:         a.text,
			FinishReason: a.finish,
		}}
		return response
	}

	response.Choices = []Choice{{
		Index: 0,
		Message: ChatMessage{
			Role:    "assistant",
			Content: a.text,
		},
		FinishReason: a.finish,
	}}
	return response
}

func extractPromptString(req OpenAIRequest) (string, error) {
	if prompt, ok := req.RawBody["prompt"]; ok {
		text, ok := prompt.(string)
		if !ok {
			return "", fmt.Errorf("prompt must be a string for backend %q", req.Model)
		}
		return text, nil
	}
	return req.Prompt, nil
}

func extractRawMessages(req OpenAIRequest) ([]map[string]interface{}, error) {
	if len(req.RawBody) == 0 {
		result := make([]map[string]interface{}, 0, len(req.Messages))
		for _, message := range req.Messages {
			item := map[string]interface{}{
				"role":    message.Role,
				"content": message.Content,
			}
			if message.Name != "" {
				item["name"] = message.Name
			}
			result = append(result, item)
		}
		return result, nil
	}

	rawMessages, ok := req.RawBody["messages"]
	if !ok {
		return nil, fmt.Errorf("messages is required")
	}

	items, ok := rawMessages.([]interface{})
	if !ok {
		return nil, fmt.Errorf("messages must be an array")
	}

	result := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		message, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("messages must contain objects")
		}

		cloned, ok := cloneJSONValue(message).(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("messages must contain objects")
		}
		result = append(result, cloned)
	}

	return result, nil
}

func normalizeMessageTextContent(content interface{}) string {
	switch typed := content.(type) {
	case string:
		return typed
	case []interface{}:
		var parts []string
		for _, item := range typed {
			block, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			blockType, _ := block["type"].(string)
			if blockType != "" && blockType != "text" && blockType != "input_text" {
				continue
			}
			text, _ := block["text"].(string)
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func jsonInt64(value interface{}) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), true
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return parsed, true
		}
		floatParsed, floatErr := typed.Float64()
		if floatErr != nil {
			return 0, false
		}
		return int64(floatParsed), true
	default:
		return 0, false
	}
}
