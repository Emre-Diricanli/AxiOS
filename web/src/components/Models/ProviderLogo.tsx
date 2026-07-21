import anthropicLogo from "@/assets/providers/anthropic.svg";
import cohereLogo from "@/assets/providers/cohere.svg";
import deepseekLogo from "@/assets/providers/deepseek.svg";
import googleLogo from "@/assets/providers/google.svg";
import groqLogo from "@/assets/providers/groq.svg";
import mistralLogo from "@/assets/providers/mistral.svg";
import ollamaLogo from "@/assets/providers/ollama.svg";
import openaiLogo from "@/assets/providers/openai.svg";
import openrouterLogo from "@/assets/providers/openrouter.svg";
import perplexityLogo from "@/assets/providers/perplexity.svg";
import togetherLogo from "@/assets/providers/together.svg";
import xaiLogo from "@/assets/providers/xai.svg";

const PROVIDER_LOGOS: Record<string, string> = {
  anthropic: anthropicLogo,
  cohere: cohereLogo,
  deepseek: deepseekLogo,
  google: googleLogo,
  gemini: googleLogo,
  groq: groqLogo,
  mistral: mistralLogo,
  ollama: ollamaLogo,
  openai: openaiLogo,
  openrouter: openrouterLogo,
  perplexity: perplexityLogo,
  together: togetherLogo,
  xai: xaiLogo,
  supergrok: xaiLogo,
};

function resolveProviderLogo(provider: string): string | undefined {
  const normalized = provider.toLowerCase().replace(/[^a-z0-9]/g, "");
  return Object.entries(PROVIDER_LOGOS).find(([key]) => normalized.includes(key))?.[1];
}

export function ProviderLogo({ provider, small = false }: { provider: string; small?: boolean }) {
  const logo = resolveProviderLogo(provider);
  const sizeClass = small ? "w-5 h-5 rounded p-0.5" : "w-9 h-9 rounded-md p-1.5";
  return (
    <span className={`${sizeClass} flex items-center justify-center shrink-0 border border-border bg-surface-raised`} aria-hidden="true">
      {logo ? (
        <img src={logo} alt="" className="w-full h-full object-contain" />
      ) : (
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" className="text-muted-foreground">
          <path d="M12 2 4 6.5v9L12 20l8-4.5v-9L12 2Z" />
          <path d="m4 6.5 8 4.5 8-4.5M12 11v9" />
        </svg>
      )}
    </span>
  );
}
