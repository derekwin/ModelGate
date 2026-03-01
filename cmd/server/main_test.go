package main

import (
	"testing"
	"time"

	"modelgate/internal/models"
)

func TestNormalizeStopSequences(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		wantLen int
		wantErr bool
	}{
		{name: "nil", input: nil, wantLen: 0},
		{name: "string", input: "END", wantLen: 1},
		{name: "array", input: []interface{}{"a", "b"}, wantLen: 2},
		{name: "typed array", input: []string{"x"}, wantLen: 1},
		{name: "invalid element", input: []interface{}{"a", 1}, wantErr: true},
		{name: "invalid type", input: 123, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeStopSequences(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Fatalf("expected %d stop sequences, got %d", tt.wantLen, len(got))
			}
		})
	}
}

func TestBuildOpenAIModelsResponse(t *testing.T) {
	now := time.Now()
	resp := buildOpenAIModelsResponse([]models.Model{
		{
			Name:        "qwen3:8b",
			BackendType: "ollama",
			BaseModel: models.BaseModel{
				CreatedAt: now,
			},
		},
	})

	if resp.Object != "list" {
		t.Fatalf("expected object=list, got %s", resp.Object)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected one model, got %d", len(resp.Data))
	}
	if resp.Data[0].ID != "qwen3:8b" {
		t.Fatalf("expected model id qwen3:8b, got %s", resp.Data[0].ID)
	}
	if resp.Data[0].OwnedBy != "ollama" {
		t.Fatalf("expected owned_by ollama, got %s", resp.Data[0].OwnedBy)
	}
	if resp.Data[0].Created != now.Unix() {
		t.Fatalf("expected created timestamp %d, got %d", now.Unix(), resp.Data[0].Created)
	}
}
