package adapters

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"modelgate/internal/models"
)

func TestOpenAIAdapterChatCompletionPassesThroughRawPayload(t *testing.T) {
	var captured map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	adapter := NewOpenAIAdapter(server.URL, "test-key", nil, time.Second, ResilienceOptions{})
	req := OpenAIRequest{
		Model: "gpt-4o-mini",
		RawBody: map[string]interface{}{
			"model": "gpt-4o-mini",
			"messages": []interface{}{
				map[string]interface{}{
					"role": "user",
					"content": []interface{}{
						map[string]interface{}{"type": "text", "text": "hello"},
						map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": "https://example.com/img.png"}},
					},
				},
			},
			"tools": []interface{}{
				map[string]interface{}{
					"type": "function",
					"function": map[string]interface{}{
						"name":        "lookup_weather",
						"description": "Lookup weather",
					},
				},
			},
			"tool_choice": "auto",
		},
	}

	resp, err := adapter.ChatCompletion(context.Background(), req, models.Model{})
	if err != nil {
		t.Fatalf("chat completion failed: %v", err)
	}

	if resp.Model != "gpt-4o-mini" {
		t.Fatalf("unexpected model: %s", resp.Model)
	}

	tools, ok := captured["tools"].([]interface{})
	if !ok || len(tools) != 1 {
		t.Fatalf("tools not forwarded: %#v", captured["tools"])
	}
	if captured["tool_choice"] != "auto" {
		t.Fatalf("tool_choice not forwarded: %#v", captured["tool_choice"])
	}

	messages, ok := captured["messages"].([]interface{})
	if !ok || len(messages) != 1 {
		t.Fatalf("messages not forwarded: %#v", captured["messages"])
	}
	firstMessage, ok := messages[0].(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected message shape: %#v", messages[0])
	}
	content, ok := firstMessage["content"].([]interface{})
	if !ok || len(content) != 2 {
		t.Fatalf("multimodal content not preserved: %#v", firstMessage["content"])
	}
}

func TestOpenAIAdapterCompletionStreamingUsesSSE(t *testing.T) {
	var captured map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"cmpl-1\",\"object\":\"text_completion\",\"created\":1,\"model\":\"gpt-3.5-turbo-instruct\",\"choices\":[{\"index\":0,\"text\":\"Hel\",\"finish_reason\":null}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"cmpl-1\",\"object\":\"text_completion\",\"created\":1,\"model\":\"gpt-3.5-turbo-instruct\",\"choices\":[{\"index\":0,\"text\":\"lo\",\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":2,\"total_tokens\":4}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	var forwarded []string
	adapter := NewOpenAIAdapter(server.URL, "test-key", nil, time.Second, ResilienceOptions{})
	req := OpenAIRequest{
		Model: "gpt-3.5-turbo-instruct",
		RawBody: map[string]interface{}{
			"model":    "gpt-3.5-turbo-instruct",
			"prompt":   "Hello",
			"stream":   true,
			"logprobs": json.Number("2"),
		},
		Stream: true,
		StreamFunc: func(chunk string) {
			forwarded = append(forwarded, chunk)
		},
	}

	resp, err := adapter.Completion(context.Background(), req, models.Model{})
	if err != nil {
		t.Fatalf("completion failed: %v", err)
	}

	if len(forwarded) != 2 {
		t.Fatalf("expected 2 forwarded chunks, got %d", len(forwarded))
	}
	if captured["logprobs"] != float64(2) {
		t.Fatalf("logprobs not forwarded: %#v", captured["logprobs"])
	}
	if got := resp.Choices[0].Text; got != "Hello" {
		t.Fatalf("unexpected aggregated text: %q", got)
	}
	if resp.Object != "text_completion" {
		t.Fatalf("unexpected object: %s", resp.Object)
	}
}

func TestOpenAIAdapterChatCompletionPreservesToolCallsInJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-tool-1","object":"chat.completion","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup_weather","arguments":"{\"city\":\"Shanghai\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`))
	}))
	defer server.Close()

	adapter := NewOpenAIAdapter(server.URL, "test-key", nil, time.Second, ResilienceOptions{})
	resp, err := adapter.ChatCompletion(context.Background(), OpenAIRequest{
		Model: "gpt-4o-mini",
		RawBody: map[string]interface{}{
			"model": "gpt-4o-mini",
			"messages": []interface{}{
				map[string]interface{}{"role": "user", "content": "weather"},
			},
		},
	}, models.Model{})
	if err != nil {
		t.Fatalf("chat completion failed: %v", err)
	}

	jsonBytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	var encoded map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &encoded); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	choices, ok := encoded["choices"].([]interface{})
	if !ok || len(choices) != 1 {
		t.Fatalf("unexpected choices: %#v", encoded["choices"])
	}
	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected choice shape: %#v", choices[0])
	}
	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected message shape: %#v", choice["message"])
	}
	toolCalls, ok := message["tool_calls"].([]interface{})
	if !ok || len(toolCalls) != 1 {
		t.Fatalf("tool_calls not preserved: %#v", message["tool_calls"])
	}
	if resp.Usage.TotalTokens != 5 {
		t.Fatalf("usage not preserved: %+v", resp.Usage)
	}
}

func TestOllamaAdapterChatStreamingEmitsOpenAIChunks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"message\":{\"role\":\"assistant\",\"content\":\"Hi\"},\"done\":false}\n"))
		_, _ = w.Write([]byte("{\"message\":{\"role\":\"assistant\",\"content\":\"\"},\"done\":true,\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":1}}\n"))
	}))
	defer server.Close()

	var chunks []string
	adapter := NewOllamaAdapter(server.URL, nil, time.Second, ResilienceOptions{})
	req := OpenAIRequest{
		Model: "llama3",
		RawBody: map[string]interface{}{
			"model": "llama3",
			"messages": []interface{}{
				map[string]interface{}{
					"role": "user",
					"content": []interface{}{
						map[string]interface{}{"type": "text", "text": "Say hi"},
					},
				},
			},
			"stream": true,
		},
		Stream: true,
		StreamFunc: func(chunk string) {
			chunks = append(chunks, chunk)
		},
	}

	resp, err := adapter.ChatCompletion(context.Background(), req, models.Model{})
	if err != nil {
		t.Fatalf("chat completion failed: %v", err)
	}

	if len(chunks) != 3 {
		t.Fatalf("expected role/content/final chunks, got %d", len(chunks))
	}
	if !strings.Contains(chunks[0], "\"object\":\"chat.completion.chunk\"") {
		t.Fatalf("first chunk missing chat chunk object: %s", chunks[0])
	}
	if !strings.Contains(chunks[0], "\"role\":\"assistant\"") {
		t.Fatalf("first chunk missing assistant role: %s", chunks[0])
	}
	if !strings.Contains(chunks[1], "\"content\":\"Hi\"") {
		t.Fatalf("content chunk missing content: %s", chunks[1])
	}
	if !strings.Contains(chunks[2], "\"finish_reason\":\"stop\"") {
		t.Fatalf("final chunk missing finish reason: %s", chunks[2])
	}
	if resp.Usage.TotalTokens != 4 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}
}
