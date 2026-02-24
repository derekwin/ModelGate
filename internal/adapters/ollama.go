package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"bufio"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	service "modelgate/internal/service"
)

type OllamaAdapter struct {
	BaseURL string
	Client  *http.Client
}

func (a *OllamaAdapter) ensureClient() *http.Client {
	if a.Client != nil {
		return a.Client
	}
	return httpClientWithTimeout(300 * time.Second)
}

func (a *OllamaAdapter) postJSON(ctx context.Context, path string, payload interface{}) (*http.Response, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(a.BaseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	log.Info().Str("backend", "ollama").Str("url", url).Msg("forwarding request to Ollama")
	return a.ensureClient().Do(req)
}

func (a *OllamaAdapter) ChatCompletion(ctx context.Context, req service.OpenAIRequest, model *service.Model) (*service.OpenAIResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()
	resp, err := a.postJSON(ctx, "/v1/chat/completions", req)
	if err != nil {
		log.Error().Err(err).Msg("Ollama chat request failed")
		return nil, &service.APIError{Message: err.Error(), Type: "http_error", Code: 0}
	}
    defer resp.Body.Close()
    // If client requested streaming, Ollama returns SSE events data: ...
    // We will attempt to translate the SSE stream into a final JSON response by
    // aggregating the content deltas from the stream. This keeps compatibility with
    // the existing gateway path which expects a single OpenAIResponse object.
    if req.Stream {
		// Read streaming data and accumulate deltas
		var builder strings.Builder
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Ollama SSE lines typically look like: data: {"choices":[...]} or data: [DONE]
			if strings.HasPrefix(line, "data:") {
				data := strings.TrimSpace(line[len("data:") :])
				if data == "[DONE]" {
					break
				}
				// The data payload is JSON; try to extract delta.content if present
				var payload map[string]interface{}
				if err := json.Unmarshal([]byte(data), &payload); err != nil {
					// ignore parse errors but continue streaming
					continue
				}
				if choices, ok := payload["choices"].([]interface{}); ok && len(choices) > 0 {
					if c, ok := choices[0].(map[string]interface{}); ok {
						if delta, ok := c["delta"].(map[string]interface{}); ok {
							if content, ok := delta["content"].(string); ok {
								builder.WriteString(content)
							}
						}
					}
				}
			}
			if err := scanner.Err(); err != nil {
				log.Error().Err(err).Msg("Ollama streaming read error")
			}
		}
		// Build final response from accumulated content
		var out service.OpenAIResponse
		out.Choices = []service.Choice{{Index: 0, Message: service.Message{Role: "assistant", Content: builder.String()}}}
		return &out, nil
    }
    // Non-stream path: read full body as JSON
    b, _ := io.ReadAll(resp.Body)
    if resp.StatusCode >= 400 {
		if apiErr := parseOllamaError(b, resp.StatusCode); apiErr != nil {
			return nil, apiErr
		}
		return nil, &service.APIError{Message: string(b), Type: "http_error", Code: resp.StatusCode}
	}
	var out service.OpenAIResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("Ollama chat decode: %w", err)
	}
	return &out, nil
}

func (a *OllamaAdapter) Completion(ctx context.Context, req service.OpenAIRequest, model *service.Model) (*service.OpenAIResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()
	resp, err := a.postJSON(ctx, "/v1/completions", req)
	if err != nil {
		log.Error().Err(err).Msg("Ollama completion request failed")
		return nil, &service.APIError{Message: err.Error(), Type: "http_error", Code: 0}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		if apiErr := parseOllamaError(b, resp.StatusCode); apiErr != nil {
			return nil, apiErr
		}
		return nil, &service.APIError{Message: string(b), Type: "http_error", Code: resp.StatusCode}
	}
	var out service.OpenAIResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("Ollama completion decode: %w", err)
	}
	return &out, nil
}

func (a *OllamaAdapter) Models(ctx context.Context, model *service.Model) (*service.OpenAIModelsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()
	url := strings.TrimRight(a.BaseURL, "/") + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	log.Info().Str("backend", "ollama").Str("url", url).Msg("requesting models from Ollama")
	resp, err := a.ensureClient().Do(req)
	if err != nil {
		log.Error().Err(err).Msg("Ollama models request failed")
		return nil, &service.APIError{Message: err.Error(), Type: "http_error", Code: 0}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		if apiErr := parseOllamaError(b, resp.StatusCode); apiErr != nil {
			return nil, apiErr
		}
		return nil, &service.APIError{Message: string(b), Type: "http_error", Code: resp.StatusCode}
	}
	var out service.OpenAIModelsResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("Ollama models decode: %w", err)
	}
	return &out, nil
}

func parseOllamaError(body []byte, status int) *service.APIError {
	var payload map[string]interface{}
	if json.Unmarshal(body, &payload) != nil {
		return nil
	}
	if errObj, ok := payload["error"].(map[string]interface{}); ok {
		msg := ""
		if v, ok := errObj["message"].(string); ok {
			msg = v
		}
		typ := ""
		if v, ok := errObj["type"].(string); ok {
			typ = v
		}
		code := status
		if v, ok := errObj["code"].(float64); ok {
			code = int(v)
		}
		return &service.APIError{Message: msg, Type: typ, Code: code}
	}
	return nil
}

func (a *OllamaAdapter) SyncModels(ctx context.Context) error {
	return nil
}

func httpClientWithTimeout(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}
