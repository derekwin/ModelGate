package admin

import "testing"

func TestBackendModelsURL(t *testing.T) {
	tests := []struct {
		name    string
		backend string
		baseURL string
		want    string
		wantErr bool
	}{
		{
			name:    "ollama basic",
			backend: "ollama",
			baseURL: "http://localhost:11434",
			want:    "http://localhost:11434/api/tags",
		},
		{
			name:    "openai with v1 suffix",
			backend: "openai",
			baseURL: "https://api.openai.com/v1",
			want:    "https://api.openai.com/v1/models",
		},
		{
			name:    "vllm without v1 suffix",
			backend: "vllm",
			baseURL: "http://localhost:8000",
			want:    "http://localhost:8000/v1/models",
		},
		{
			name:    "unsupported backend",
			backend: "unknown",
			baseURL: "http://localhost:8000",
			wantErr: true,
		},
		{
			name:    "empty base url",
			backend: "vllm",
			baseURL: " ",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := backendModelsURL(tt.backend, tt.baseURL)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestIsSupportedBackend(t *testing.T) {
	if !isSupportedBackend("OpenAI") {
		t.Fatalf("expected OpenAI to be supported")
	}
	if isSupportedBackend("custom-backend") {
		t.Fatalf("expected custom-backend to be unsupported")
	}
}
