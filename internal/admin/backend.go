package admin

import (
	"fmt"
	"strings"
)

var supportedBackends = map[string]struct{}{
	"ollama":   {},
	"vllm":     {},
	"llamacpp": {},
	"openai":   {},
	"api3":     {},
}

func isSupportedBackend(backend string) bool {
	_, ok := supportedBackends[strings.ToLower(strings.TrimSpace(backend))]
	return ok
}

func backendModelsURL(backend, baseURL string) (string, error) {
	normalizedBackend := strings.ToLower(strings.TrimSpace(backend))
	normalizedBaseURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if normalizedBaseURL == "" {
		return "", fmt.Errorf("base url is empty")
	}

	switch normalizedBackend {
	case "ollama":
		return normalizedBaseURL + "/api/tags", nil
	case "vllm", "openai", "llamacpp", "api3":
		if strings.HasSuffix(normalizedBaseURL, "/v1") {
			return normalizedBaseURL + "/models", nil
		}
		return normalizedBaseURL + "/v1/models", nil
	default:
		return "", fmt.Errorf("unsupported backend: %s", backend)
	}
}
