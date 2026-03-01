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
	return &AdapterFactory{
		ollama:   NewOllamaAdapter(cfg.Adapters.Ollama.BaseURL, int64(cfg.Timeout.Seconds())),
		vllm:     NewVLLMAdapter(cfg.Adapters.VLLM.BaseURL, int64(cfg.Timeout.Seconds())),
		llamacpp: NewLlamaCppAdapter(cfg.Adapters.LlamaCPP.BaseURL, int64(cfg.Timeout.Seconds())),
		openai:   NewOpenAIAdapter(cfg.Adapters.OpenAI.BaseURL, "", int64(cfg.Timeout.Seconds())),
		api3:     NewAPI3Adapter(cfg.Adapters.API3.BaseURL, "", int64(cfg.Timeout.Seconds())),
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
