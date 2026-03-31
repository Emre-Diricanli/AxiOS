import { useState } from "react";
import { useProviders } from "@/hooks/useProviders";
import type { CloudProvider } from "@/types/providers";

/* ── Provider brand colors ─────────────────────────────── */

const PROVIDER_COLORS: Record<string, string> = {
  anthropic: "bg-orange-500",
  openai: "bg-green-500",
  google: "bg-blue-500",
  mistral: "bg-orange-600",
  groq: "bg-purple-500",
  together: "bg-cyan-500",
  openrouter: "bg-indigo-500",
  deepseek: "bg-blue-600",
  xai: "bg-gray-400",
  cohere: "bg-red-400",
  perplexity: "bg-teal-500",
};

function getProviderColor(name: string): string {
  const key = name.toLowerCase();
  for (const [k, v] of Object.entries(PROVIDER_COLORS)) {
    if (key.includes(k)) return v;
  }
  return "bg-primary";
}

/* ── Provider Logo (first-letter circle) ───────────────── */

function ProviderLogo({ name }: { name: string }) {
  const color = getProviderColor(name);
  return (
    <div
      className={`w-9 h-9 rounded-xl ${color} flex items-center justify-center shrink-0`}
    >
      <span className="text-sm font-bold text-white leading-none">
        {name.charAt(0).toUpperCase()}
      </span>
    </div>
  );
}

/* ── Provider Card ─────────────────────────────────────── */

function ProviderCard({
  provider,
  onSetKey,
  onRemoveKey,
  onActivate,
}: {
  provider: CloudProvider;
  onSetKey: (key: string) => Promise<void>;
  onRemoveKey: () => Promise<void>;
  onActivate: (model: string) => Promise<void>;
}) {
  const [apiKey, setApiKey] = useState("");
  const [showKey, setShowKey] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [selectedModel, setSelectedModel] = useState(provider.models[0] ?? "");
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [cardError, setCardError] = useState<string | null>(null);

  const handleConnect = async () => {
    if (!apiKey.trim()) return;
    setSubmitting(true);
    setCardError(null);
    try {
      await onSetKey(apiKey.trim());
      setApiKey("");
      setShowKey(false);
    } catch (err) {
      setCardError(err instanceof Error ? err.message : "Failed to connect");
    } finally {
      setSubmitting(false);
    }
  };

  const handleActivate = async () => {
    if (!selectedModel) return;
    setSubmitting(true);
    setCardError(null);
    try {
      await onActivate(selectedModel);
    } catch (err) {
      setCardError(err instanceof Error ? err.message : "Failed to activate");
    } finally {
      setSubmitting(false);
    }
  };

  const handleRemove = async () => {
    setSubmitting(true);
    setCardError(null);
    try {
      await onRemoveKey();
      setConfirmRemove(false);
    } catch (err) {
      setCardError(err instanceof Error ? err.message : "Failed to remove key");
    } finally {
      setSubmitting(false);
    }
  };

  const borderClass = provider.active
    ? "border-glow glow-sm"
    : "";
  const dimClass = !provider.has_key ? "opacity-70" : "";

  return (
    <div
      className={`glass rounded-xl p-4 flex flex-col gap-3 transition-all duration-200 hover:bg-accent/30 ${borderClass} ${dimClass}`}
    >
      {/* Header */}
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2.5 min-w-0">
          <ProviderLogo name={provider.name} />
          <div className="min-w-0">
            <h4 className="text-sm font-bold text-foreground truncate">
              {provider.name}
            </h4>
            <span className="text-[10px] text-muted-foreground font-mono">
              {provider.compatible}
            </span>
          </div>
        </div>
        <div className="flex items-center gap-1.5 shrink-0">
          {provider.active && (
            <span className="px-2 py-0.5 rounded-full text-[9px] font-semibold uppercase tracking-wider bg-primary/15 text-primary border border-primary/25 glow-sm">
              Active
            </span>
          )}
          {provider.has_key && !provider.active && (
            <span className="px-2 py-0.5 rounded-full text-[9px] font-medium bg-green-500/15 text-green-400 border border-green-500/25">
              Connected
            </span>
          )}
        </div>
      </div>

      {/* Configured state: model selector + actions */}
      {provider.has_key && (
        <>
          <div className="flex items-center gap-2">
            <div className="relative flex-1 min-w-0">
              <select
                value={selectedModel}
                onChange={(e) => setSelectedModel(e.target.value)}
                className="w-full px-2 py-1.5 pr-7 rounded-lg text-[11px] font-mono bg-secondary text-foreground border border-border focus:outline-none focus:ring-1 focus:ring-primary/50 appearance-none cursor-pointer"
              >
                {(provider.models ?? []).map((m) => (
                  <option key={m} value={m}>
                    {m}
                  </option>
                ))}
              </select>
              <svg className="absolute right-2 top-1/2 -translate-y-1/2 pointer-events-none text-muted-foreground" width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
                <path d="M6 9l6 6 6-6" />
              </svg>
            </div>
            {!provider.active && (
              <button
                onClick={handleActivate}
                disabled={submitting || !selectedModel}
                className="px-3 py-1.5 rounded-lg text-xs font-medium bg-primary/15 text-primary border border-primary/25 hover:bg-primary/25 transition-colors disabled:opacity-50 shrink-0"
              >
                Use
              </button>
            )}
          </div>

          {/* Remove key */}
          <div className="flex items-center mt-auto pt-0.5">
            {confirmRemove ? (
              <div className="flex items-center gap-1.5">
                <span className="text-[10px] text-destructive whitespace-nowrap">
                  Remove key?
                </span>
                <button
                  onClick={() => setConfirmRemove(false)}
                  className="px-2 py-1 rounded-md text-[10px] font-medium bg-secondary text-muted-foreground hover:bg-secondary/80 transition-colors"
                >
                  Cancel
                </button>
                <button
                  onClick={handleRemove}
                  disabled={submitting}
                  className="px-2 py-1 rounded-md text-[10px] font-medium bg-destructive/15 text-destructive border border-destructive/25 hover:bg-destructive/25 transition-colors disabled:opacity-50"
                >
                  Remove
                </button>
              </div>
            ) : (
              <button
                onClick={() => setConfirmRemove(true)}
                className="text-[10px] text-muted-foreground hover:text-destructive transition-colors"
              >
                Remove Key
              </button>
            )}
          </div>
        </>
      )}

      {/* Unconfigured state: API key input */}
      {!provider.has_key && (
        <div className="flex items-center gap-2">
          <div className="relative flex-1 min-w-0">
            <input
              type={showKey ? "text" : "password"}
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleConnect();
              }}
              placeholder="API Key"
              className="w-full px-3 py-2 pr-8 rounded-lg text-xs font-mono glass-subtle text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary/50"
            />
            <button
              type="button"
              onClick={() => setShowKey(!showKey)}
              className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
              tabIndex={-1}
            >
              {showKey ? (
                <svg
                  width="14"
                  height="14"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                >
                  <path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94" />
                  <path d="M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19" />
                  <line x1="1" y1="1" x2="23" y2="23" />
                </svg>
              ) : (
                <svg
                  width="14"
                  height="14"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                >
                  <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
                  <circle cx="12" cy="12" r="3" />
                </svg>
              )}
            </button>
          </div>
          <button
            onClick={handleConnect}
            disabled={submitting || !apiKey.trim()}
            className="px-3 py-2 rounded-lg text-xs font-medium bg-primary/15 text-primary border border-primary/25 hover:bg-primary/25 transition-colors disabled:opacity-50 shrink-0"
          >
            {submitting ? "..." : "Connect"}
          </button>
        </div>
      )}

      {/* Error */}
      {cardError && (
        <div className="rounded-lg bg-destructive/10 border border-destructive/20 px-3 py-1.5 text-[10px] text-destructive">
          {cardError}
        </div>
      )}
    </div>
  );
}

/* ── Providers Panel ───────────────────────────────────── */

export function ProvidersPanel() {
  const {
    providers,
    loading,
    error,
    setAPIKey,
    removeAPIKey,
    activateProvider,
  } = useProviders();

  const configuredCount = providers.filter((p) => p.has_key).length;

  // Sort: configured providers first, then unconfigured; active at the very top
  const sorted = [...providers].sort((a, b) => {
    if (a.active !== b.active) return a.active ? -1 : 1;
    if (a.has_key !== b.has_key) return a.has_key ? -1 : 1;
    return a.name.localeCompare(b.name);
  });

  if (loading) {
    return (
      <div>
        <div className="flex items-center justify-between mb-4">
          <div>
            <h2 className="text-base font-semibold tracking-tight text-foreground">
              Cloud Providers
            </h2>
            <p className="text-[11px] text-muted-foreground mt-0.5">
              Loading providers...
            </p>
          </div>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {[1, 2, 3].map((i) => (
            <div key={i} className="glass rounded-xl p-4 h-32 animate-pulse" />
          ))}
        </div>
      </div>
    );
  }

  return (
    <div>
      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-3">
          <div>
            <div className="flex items-center gap-2">
              <h2 className="text-base font-semibold tracking-tight text-foreground">
                Cloud Providers
              </h2>
              <span className="px-2 py-0.5 rounded-full text-[10px] font-mono bg-secondary text-muted-foreground">
                {configuredCount} configured
              </span>
            </div>
            <p className="text-[11px] text-muted-foreground mt-0.5">
              Connect your API keys to use cloud models
            </p>
          </div>
        </div>
      </div>

      {/* Error */}
      {error && (
        <div className="rounded-xl bg-destructive/10 border border-destructive/20 px-4 py-3 text-xs text-destructive mb-4">
          {error}
        </div>
      )}

      {/* Provider grid */}
      {sorted.length === 0 ? (
        <div className="glass rounded-xl p-6 flex flex-col items-center justify-center text-center">
          <div className="w-12 h-12 rounded-2xl glass flex items-center justify-center mb-3 glow-primary">
            <svg
              width="20"
              height="20"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
              strokeLinecap="round"
              strokeLinejoin="round"
              className="text-primary"
            >
              <path d="M22 12h-4l-3 9L9 3l-3 9H2" />
            </svg>
          </div>
          <h3 className="text-sm font-medium text-foreground mb-1">
            No providers available
          </h3>
          <p className="text-xs text-muted-foreground">
            Cloud API providers will appear here once configured on the server
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4 gap-4">
          {sorted.map((provider) => (
            <ProviderCard
              key={provider.id}
              provider={provider}
              onSetKey={(key) => setAPIKey(provider.id, key)}
              onRemoveKey={() => removeAPIKey(provider.id)}
              onActivate={(model) => activateProvider(provider.id, model)}
            />
          ))}
        </div>
      )}
    </div>
  );
}
