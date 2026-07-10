package providers

import "testing"

func TestNormalizeModelForProvider(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		provider string
		want     string
	}{
		// Aggregators want vendor/model slugs.
		{"openrouter adds anthropic vendor", "claude-sonnet-4-6", "openrouter", "anthropic/claude-sonnet-4-6"},
		{"openrouter adds openai vendor", "gpt-4o", "openrouter", "openai/gpt-4o"},
		{"openrouter adds openai vendor for o1", "o1-mini", "openrouter", "openai/o1-mini"},
		{"openrouter adds google vendor", "gemini-2.5-pro", "openrouter", "google/gemini-2.5-pro"},
		{"openrouter adds meta vendor", "llama-3.1-70b-instruct", "openrouter", "meta-llama/llama-3.1-70b-instruct"},
		{"openrouter adds mistral vendor", "mixtral-8x7b", "openrouter", "mistralai/mixtral-8x7b"},
		{"openrouter adds xai vendor", "grok-2", "openrouter", "x-ai/grok-2"},
		{"openrouter keeps existing slug", "anthropic/claude-sonnet-4", "openrouter", "anthropic/claude-sonnet-4"},
		{"openrouter passes unknown bare name through", "some-model", "openrouter", "some-model"},

		// Native APIs want bare names.
		{"anthropic strips vendor slug", "anthropic/claude-sonnet-4-6", "anthropic", "claude-sonnet-4-6"},
		{"openai strips vendor slug", "openai/gpt-4o", "openai", "gpt-4o"},
		{"deepseek strips vendor slug", "deepseek/deepseek-chat", "deepseek", "deepseek-chat"},
		{"anthropic keeps bare name", "claude-sonnet-4-6", "anthropic", "claude-sonnet-4-6"},
		{"provider lookup is case-insensitive", "openai/gpt-4o", "OpenAI", "gpt-4o"},

		// Providers with legitimate slashes in model names pass through.
		{"together keeps full slug", "meta-llama/Llama-3.1-70B-Instruct", "together", "meta-llama/Llama-3.1-70B-Instruct"},
		{"fireworks keeps account path", "accounts/fireworks/models/llama-v3p1-70b-instruct", "fireworks", "accounts/fireworks/models/llama-v3p1-70b-instruct"},
		{"ollama keeps tag names", "llama3.1:8b", "ollama", "llama3.1:8b"},
		{"custom passes through", "vendor/model", "custom", "vendor/model"},

		{"empty model passes through", "", "openrouter", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeModelForProvider(tt.model, tt.provider); got != tt.want {
				t.Errorf("NormalizeModelForProvider(%q, %q) = %q, want %q", tt.model, tt.provider, got, tt.want)
			}
		})
	}
}
