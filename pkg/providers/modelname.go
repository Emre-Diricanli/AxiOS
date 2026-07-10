package providers

import "strings"

// vendorSlugPrefix maps bare-model-name prefixes to the vendor slug used by
// aggregators such as OpenRouter (which address models as "vendor/model").
var vendorSlugPrefix = []struct {
	modelPrefix string
	vendor      string
}{
	{"claude", "anthropic"},
	{"gpt", "openai"},
	{"chatgpt", "openai"},
	{"o1", "openai"},
	{"o3", "openai"},
	{"o4", "openai"},
	{"gemini", "google"},
	{"gemma", "google"},
	{"llama", "meta-llama"},
	{"mistral", "mistralai"},
	{"mixtral", "mistralai"},
	{"codestral", "mistralai"},
	{"deepseek", "deepseek"},
	{"grok", "x-ai"},
	{"command", "cohere"},
	{"qwen", "qwen"},
}

// bareNameProviders are native APIs that want bare model names; a
// "vendor/model" slug is stripped down to the model segment for them.
// Together, Fireworks, OpenRouter, and Ollama legitimately use slashes in
// model identifiers and are deliberately absent.
var bareNameProviders = map[string]bool{
	"anthropic": true,
	"openai":    true,
	"google":    true,
	"mistral":   true,
	"groq":      true,
	"deepseek":  true,
	"xai":       true,
}

// NormalizeModelForProvider adapts a model identifier to a provider's naming
// convention: aggregators (openrouter) want "vendor/model" slugs, native APIs
// want bare names. Unknown combinations pass through unchanged.
func NormalizeModelForProvider(model, provider string) string {
	if model == "" {
		return model
	}
	provider = strings.ToLower(provider)

	switch {
	case provider == "openrouter":
		if strings.Contains(model, "/") {
			return model
		}
		lower := strings.ToLower(model)
		for _, e := range vendorSlugPrefix {
			if strings.HasPrefix(lower, e.modelPrefix) {
				return e.vendor + "/" + model
			}
		}
		return model

	case bareNameProviders[provider]:
		if idx := strings.LastIndex(model, "/"); idx >= 0 {
			return model[idx+1:]
		}
		return model

	default:
		return model
	}
}
