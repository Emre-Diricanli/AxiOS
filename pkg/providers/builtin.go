package providers

import "strings"

// anthropicOAuthPrefix marks OAuth tokens from `claude setup-token`, which
// authenticate with Authorization: Bearer instead of x-api-key.
const anthropicOAuthPrefix = "sk-ant-oat01-"

// anthropicAuthHeader picks the Anthropic auth header by token shape:
// OAuth tokens (sk-ant-oat01-...) use Authorization: Bearer, API keys use x-api-key.
func anthropicAuthHeader(key string) (string, string) {
	if strings.HasPrefix(key, anthropicOAuthPrefix) {
		return "Authorization", "Bearer " + key
	}
	return "x-api-key", key
}

// Builtins returns fresh copies of every built-in provider profile. BaseURLs
// include the API version prefix; transports append only the final path
// segment (/chat/completions, /messages, /api/chat).
func Builtins() []*Profile {
	return []*Profile{
		{
			Name:         "anthropic",
			Aliases:      []string{"claude"},
			APIMode:      APIModeAnthropicMessages,
			BaseURL:      "https://api.anthropic.com/v1",
			EnvVars:      []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"},
			DefaultModel: "claude-sonnet-4-6",
			FallbackModels: []string{
				"claude-opus-4-6", "claude-haiku-4-5",
			},
			DefaultHeaders: map[string]string{"anthropic-version": "2023-06-01"},
			AuthHeader:     anthropicAuthHeader,
		},
		{
			Name:         "openai",
			APIMode:      APIModeChatCompletions,
			BaseURL:      "https://api.openai.com/v1",
			EnvVars:      []string{"OPENAI_API_KEY"},
			DefaultModel: "gpt-4o",
			FallbackModels: []string{
				"gpt-4o-mini", "gpt-4-turbo", "o1", "o1-mini", "o3-mini",
			},
		},
		{
			Name:         "openrouter",
			APIMode:      APIModeChatCompletions,
			BaseURL:      "https://openrouter.ai/api/v1",
			EnvVars:      []string{"OPENROUTER_API_KEY"},
			DefaultModel: "anthropic/claude-sonnet-4",
			FallbackModels: []string{
				"openai/gpt-4o", "google/gemini-2.5-pro", "meta-llama/llama-3.1-70b-instruct",
			},
		},
		{
			Name:         "groq",
			APIMode:      APIModeChatCompletions,
			BaseURL:      "https://api.groq.com/openai/v1",
			EnvVars:      []string{"GROQ_API_KEY"},
			DefaultModel: "llama-3.1-70b-versatile",
			FallbackModels: []string{
				"llama-3.1-8b-instant", "mixtral-8x7b-32768", "gemma2-9b-it",
			},
		},
		{
			Name:         "deepseek",
			APIMode:      APIModeChatCompletions,
			BaseURL:      "https://api.deepseek.com/v1",
			EnvVars:      []string{"DEEPSEEK_API_KEY"},
			DefaultModel: "deepseek-chat",
			FallbackModels: []string{
				"deepseek-coder", "deepseek-reasoner",
			},
		},
		{
			Name:           "xai",
			Aliases:        []string{"grok"},
			APIMode:        APIModeChatCompletions,
			BaseURL:        "https://api.x.ai/v1",
			EnvVars:        []string{"XAI_API_KEY"},
			DefaultModel:   "grok-2",
			FallbackModels: []string{"grok-2-mini"},
		},
		{
			Name:         "mistral",
			APIMode:      APIModeChatCompletions,
			BaseURL:      "https://api.mistral.ai/v1",
			EnvVars:      []string{"MISTRAL_API_KEY"},
			DefaultModel: "mistral-large-latest",
			FallbackModels: []string{
				"mistral-medium-latest", "mistral-small-latest", "codestral-latest",
			},
		},
		{
			Name:         "together",
			APIMode:      APIModeChatCompletions,
			BaseURL:      "https://api.together.xyz/v1",
			EnvVars:      []string{"TOGETHER_API_KEY", "TOGETHERAI_API_KEY"},
			DefaultModel: "meta-llama/Llama-3.1-70B-Instruct",
			FallbackModels: []string{
				"meta-llama/Llama-3.1-8B-Instruct", "mistralai/Mixtral-8x7B-Instruct-v0.1",
			},
		},
		{
			Name:         "fireworks",
			APIMode:      APIModeChatCompletions,
			BaseURL:      "https://api.fireworks.ai/inference/v1",
			EnvVars:      []string{"FIREWORKS_API_KEY"},
			DefaultModel: "accounts/fireworks/models/llama-v3p1-70b-instruct",
			FallbackModels: []string{
				"accounts/fireworks/models/llama-v3p1-8b-instruct",
			},
		},
		{
			// Google via its OpenAI-compatible endpoint.
			Name:         "google",
			Aliases:      []string{"gemini"},
			APIMode:      APIModeChatCompletions,
			BaseURL:      "https://generativelanguage.googleapis.com/v1beta/openai",
			EnvVars:      []string{"GEMINI_API_KEY", "GOOGLE_API_KEY"},
			DefaultModel: "gemini-2.5-pro",
			FallbackModels: []string{
				"gemini-2.5-flash", "gemini-2.0-flash",
			},
		},
		{
			// BaseURL is a local default; the daemon overrides it from HostStore.
			Name:         "ollama",
			APIMode:      APIModeOllama,
			BaseURL:      "http://127.0.0.1:11434",
			DefaultModel: "llama3.1:8b",
		},
		{
			// OpenAI-compatible endpoint with a user-supplied base URL.
			Name:    "custom",
			APIMode: APIModeChatCompletions,
		},
	}
}

func init() {
	for _, p := range Builtins() {
		Register(p)
	}
}
