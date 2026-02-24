package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

type APIError struct {
	Message string
	Type    string
	Code    int
}

func (e *APIError) Error() string { return e.Message }

type Model struct {
	Name             string
	BackendType      string
	BackendModelName string
}

type BackendConfig struct {
	Type      string
	URL       string
	ModelName string
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIRequest struct {
	Model       string      `json:"model"`
	Messages    []Message   `json:"messages"`
	Stream      bool        `json:"stream"`
	MaxTokens   int         `json:"max_tokens"`
	Temperature float64     `json:"temperature"`
	TopP        float64     `json:"top_p"`
	N           int         `json:"n"`
	Stop        []string    `json:"stop"`
	Functions   []Function  `json:"functions,omitempty"`
}

type Function struct {
	Name       string      `json:"name"`
	Parameters interface{} `json:"parameters"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
	Logprobs     any     `json:"logprobs"`
}

type OpenAIResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type OpenAIModelsResponse struct {
	Object string      `json:"object"`
	Data   []ModelInfo `json:"data"`
}

type ModelInfo struct {
	ID     string `json:"id"`
	Object string `json:"object"`
}

type Adapter interface {
	ChatCompletion(ctx context.Context, req OpenAIRequest, model *Model) (*OpenAIResponse, error)
	Completion(ctx context.Context, req OpenAIRequest, model *Model) (*OpenAIResponse, error)
	Models(ctx context.Context, model *Model) (*OpenAIModelsResponse, error)
	SyncModels(ctx context.Context) error
}

type GatewayService struct {
	adapters      map[string]Adapter
	models        map[string]*Model
	backendConfig map[string]BackendConfig
	backendURLs   map[string]string
	modelsSynced  map[string]bool
	modelsMutex   sync.RWMutex
}

func NewGatewayService(adapters map[string]Adapter, backendConfig map[string]BackendConfig) *GatewayService {
	backendURLs := make(map[string]string)
	for backendType, cfg := range backendConfig {
		backendURLs[backendType] = cfg.URL
	}
	return &GatewayService{
		adapters:      adapters,
		models:        make(map[string]*Model),
		backendConfig: backendConfig,
		backendURLs:   backendURLs,
		modelsSynced:  make(map[string]bool),
	}
}

func (gs *GatewayService) GetModel(ctx context.Context, name string) (*Model, error) {
	gs.modelsMutex.RLock()
	defer gs.modelsMutex.RUnlock()
	if m, ok := gs.models[name]; ok {
		return m, nil
	}
	return nil, &APIError{Message: "model not found", Type: "invalid_request_error", Code: http.StatusNotFound}
}

func (gs *GatewayService) ListModels(ctx context.Context) ([]Model, error) {
	gs.modelsMutex.RLock()
	defer gs.modelsMutex.RUnlock()
	out := make([]Model, 0, len(gs.models))
	for _, m := range gs.models {
		out = append(out, *m)
	}
	return out, nil
}

func (gs *GatewayService) UpdateUsedTokens(ctx context.Context, apiKey string, tokens int) error {
	log.Ctx(ctx).Info().Str("api_key", apiKey).Int("tokens", tokens).Msg("update used tokens (atomic would run in a transaction)")
	return nil
}

func (gs *GatewayService) Route(ctx context.Context, req OpenAIRequest) (*OpenAIResponse, error) {
	gs.modelsMutex.RLock()
	m, ok := gs.models[req.Model]
	gs.modelsMutex.RUnlock()
	if !ok {
		return nil, &APIError{Message: "model not found", Type: "invalid_request_error", Code: http.StatusNotFound}
	}
	model := *m
	adapter := gs.adapters[model.BackendType]
	if adapter == nil {
		return nil, &APIError{Message: "unsupported backend", Type: "internal_error", Code: http.StatusInternalServerError}
	}
	resp, err := adapter.ChatCompletion(ctx, req, &model)
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}
	if resp != nil {
		log.Ctx(ctx).Info().Str("model", resp.Model).Msg("gateway response prepared")
	}
	return resp, nil
}

func (gs *GatewayService) SyncModels(ctx context.Context) error {
	gs.modelsMutex.Lock()
	defer gs.modelsMutex.Unlock()

	for backendType := range gs.backendURLs {
		if gs.modelsSynced[backendType] {
			continue
		}
		if adapter, ok := gs.adapters[backendType]; ok {
			resp, err := adapter.Models(ctx, nil)
			if err != nil {
				log.Error().Err(err).Str("backend", backendType).Msg("failed to sync models")
				continue
			}
			for _, mi := range resp.Data {
				gs.models[mi.ID] = &Model{
					Name:             mi.ID,
					BackendType:      backendType,
					BackendModelName: mi.ID,
				}
			}
			log.Info().Str("backend", backendType).Int("models", len(resp.Data)).Msg("synced models from backend")
			gs.modelsSynced[backendType] = true
		}
	}
	return nil
}

func (gs *GatewayService) SetBackendConfig(backendConfig map[string]BackendConfig) {
	gs.modelsMutex.Lock()
	defer gs.modelsMutex.Unlock()

	gs.backendConfig = backendConfig
	gs.backendURLs = make(map[string]string)
	for backendType, cfg := range backendConfig {
		gs.backendURLs[backendType] = cfg.URL
	}
	gs.modelsSynced = make(map[string]bool)
	gs.models = make(map[string]*Model)
}

func Route(router *gin.Engine, gateway *GatewayService) {
	v1 := router.Group("/v1")
	{
		v1.POST("/chat/completions", handleChatCompletions)
		v1.POST("/completions", handleChatCompletions)
		v1.GET("/models", handleModels)
	}
}

func handleChatCompletions(c *gin.Context) {
	gateway := c.MustGet("gateway").(*GatewayService)
	var req OpenAIRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, map[string]interface{}{"error": map[string]interface{}{"message": err.Error(), "type": "invalid_request_error", "code": http.StatusBadRequest}})
		return
	}

	ctx := c.Request.Context()
	resp, err := gateway.Route(ctx, req)
	if err != nil {
		if apiErr, ok := err.(*APIError); ok {
			c.JSON(apiErr.Code, map[string]interface{}{"error": map[string]interface{}{"message": apiErr.Message, "type": apiErr.Type, "code": apiErr.Code}})
			return
		}
		c.JSON(http.StatusInternalServerError, map[string]interface{}{"error": map[string]interface{}{"message": err.Error(), "type": "internal_error", "code": http.StatusInternalServerError}})
		return
	}

	if req.Stream {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.WriteHeader(http.StatusOK)
		data, _ := json.Marshal(resp)
		c.Writer.Write([]byte("data: "))
		c.Writer.Write(data)
		c.Writer.Write([]byte("\n\n"))
		if fl, ok := c.Writer.(http.Flusher); ok {
			fl.Flush()
		}
		return
	}

	c.JSON(http.StatusOK, resp)
}

func handleModels(c *gin.Context) {
	gateway := c.MustGet("gateway").(*GatewayService)
	ctx := c.Request.Context()
	models, err := gateway.ListModels(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, map[string]interface{}{"error": map[string]interface{}{"message": err.Error(), "type": "internal_error", "code": http.StatusInternalServerError}})
		return
	}
	modelInfoList := make([]ModelInfo, len(models))
	for i, m := range models {
		modelInfoList[i] = ModelInfo{ID: m.Name, Object: "model"}
	}
	c.JSON(http.StatusOK, OpenAIModelsResponse{Data: modelInfoList})
}
