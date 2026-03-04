package adapters

import (
	"modelgate/internal/config"
	"modelgate/internal/models"
)

type AdapterFactory struct {
	ollama   *OllamaAdapter
	vllm     *VLLMAdapter
	llamacpp *LlamaCppAdapter
	openai   *OpenAIAdapter
	api3     *API3Adapter
}

func NewAdapterFactory(cfg *config.Config) *AdapterFactory {
	resilience := ResilienceOptions{
		RetryAttempts:       cfg.Resilience.RetryAttempts,
		RetryBackoff:        cfg.Resilience.RetryBackoff,
		FailureThreshold:    cfg.Resilience.CircuitBreaker.FailureThreshold,
		OpenTimeout:         cfg.Resilience.CircuitBreaker.OpenTimeout,
		HalfOpenMaxRequests: cfg.Resilience.CircuitBreaker.HalfOpenMaxRequests,
	}

	return &AdapterFactory{
		ollama: NewOllamaAdapter(
			cfg.Adapters.Ollama.BaseURL,
			cfg.Adapters.Ollama.FallbackURLs,
			cfg.Timeout,
			resilience,
		),
		vllm: NewVLLMAdapter(
			cfg.Adapters.VLLM.BaseURL,
			cfg.Adapters.VLLM.FallbackURLs,
			cfg.Timeout,
			resilience,
		),
		llamacpp: NewLlamaCppAdapter(
			cfg.Adapters.LlamaCPP.BaseURL,
			cfg.Adapters.LlamaCPP.FallbackURLs,
			cfg.Timeout,
			resilience,
		),
		openai: NewOpenAIAdapter(
			cfg.Adapters.OpenAI.BaseURL,
			"",
			cfg.Adapters.OpenAI.FallbackURLs,
			cfg.Timeout,
			resilience,
		),
		api3: NewAPI3Adapter(
			cfg.Adapters.API3.BaseURL,
			"",
			cfg.Adapters.API3.FallbackURLs,
			cfg.Timeout,
			resilience,
		),
	}
}

func (f *AdapterFactory) GetAdapter(backendType string) Adapter {
	switch backendType {
	case "ollama":
		return f.ollama
	case "vllm":
		return f.vllm
	case "llamacpp":
		return f.llamacpp
	case "openai":
		return f.openai
	case "api3":
		return f.api3
	default:
		return nil
	}
}

func (f *AdapterFactory) GetAdapterForModel(model models.Model) Adapter {
	switch model.BackendType {
	case "ollama":
		return f.ollama
	case "vllm":
		return f.vllm
	case "llamacpp":
		return f.llamacpp
	case "openai":
		return f.openai
	case "api3":
		return f.api3
	default:
		return nil
	}
}
