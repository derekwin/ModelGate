package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDecodeRawRequestUsesJSONNumbers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(`{"max_tokens":128,"temperature":0.5}`))

	payload, err := decodeRawRequest(ctx)
	if err != nil {
		t.Fatalf("decodeRawRequest failed: %v", err)
	}

	if _, ok := payload["max_tokens"].(json.Number); !ok {
		t.Fatalf("max_tokens should decode as json.Number, got %#v", payload["max_tokens"])
	}
}

func TestResolveModelNameFallsBackToDefault(t *testing.T) {
	modelName, err := resolveModelName(map[string]interface{}{}, "gpt-4o-mini")
	if err != nil {
		t.Fatalf("resolveModelName failed: %v", err)
	}
	if modelName != "gpt-4o-mini" {
		t.Fatalf("unexpected model name: %s", modelName)
	}
}

func TestExtractMaxTokensPrefersMaxCompletionTokens(t *testing.T) {
	maxTokens, err := extractMaxTokens(map[string]interface{}{
		"max_tokens":            json.Number("64"),
		"max_completion_tokens": json.Number("96"),
	})
	if err != nil {
		t.Fatalf("extractMaxTokens failed: %v", err)
	}
	if maxTokens != 96 {
		t.Fatalf("unexpected max tokens: %d", maxTokens)
	}
}
