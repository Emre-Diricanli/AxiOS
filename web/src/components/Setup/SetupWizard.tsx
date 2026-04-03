import { useState, useEffect, useCallback, useRef } from "react";
import type { SystemStats } from "@/types/system";

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

interface SetupWizardProps {
  onComplete: () => void;
}

type Profile =
  | "development"
  | "aiml"
  | "media"
  | "content"
  | "homeserver"
  | "general";

interface ProfileCard {
  id: Profile;
  icon: string;
  title: string;
  description: string;
}

const PROFILES: ProfileCard[] = [
  {
    id: "development",
    icon: "\u{1F5A5}\uFE0F",
    title: "Development",
    description: "Code, build, deploy",
  },
  {
    id: "aiml",
    icon: "\u{1F9E0}",
    title: "AI / ML",
    description: "Model training, inference",
  },
  {
    id: "media",
    icon: "\u{1F3AC}",
    title: "Media",
    description: "Video, audio, image processing",
  },
  {
    id: "content",
    icon: "\u{1F4DD}",
    title: "Content",
    description: "Writing, docs, publishing",
  },
  {
    id: "homeserver",
    icon: "\u{1F3E0}",
    title: "Home Server",
    description: "NAS, media streaming, backups",
  },
  {
    id: "general",
    icon: "\u{1F527}",
    title: "General",
    description: "Everyday computing",
  },
];

const CLOUD_PROVIDERS = [
  { value: "anthropic", label: "Anthropic" },
  { value: "openai", label: "OpenAI" },
  { value: "google", label: "Google Gemini" },
  { value: "groq", label: "Groq" },
  { value: "mistral", label: "Mistral" },
];

const TOTAL_STEPS = 5;

/* ------------------------------------------------------------------ */
/*  Helpers                                                            */
/* ------------------------------------------------------------------ */

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
}

function getModelRecommendation(totalBytes: number): string {
  const gb = totalBytes / (1024 * 1024 * 1024);
  if (gb < 8) return "qwen2.5:3b";
  if (gb <= 16) return "qwen2.5:7b";
  return "qwen2.5:14b";
}

/* ------------------------------------------------------------------ */
/*  Sub-components                                                     */
/* ------------------------------------------------------------------ */

function ProgressDots({ current }: { current: number }) {
  return (
    <div className="flex items-center gap-3">
      {Array.from({ length: TOTAL_STEPS }, (_, i) => {
        const step = i + 1;
        const isActive = step === current;
        const isDone = step < current;
        return (
          <div
            key={step}
            className={`h-2.5 rounded-full transition-all duration-500 ${
              isActive
                ? "w-8 bg-primary shadow-[0_0_12px_rgba(99,102,241,0.6)]"
                : isDone
                  ? "w-2.5 bg-primary/60"
                  : "w-2.5 bg-white/10"
            }`}
          />
        );
      })}
    </div>
  );
}

function Spinner({ className = "" }: { className?: string }) {
  return (
    <div className={`relative ${className}`}>
      <div className="absolute inset-0 rounded-full border-2 border-primary/20" />
      <div className="absolute inset-0 rounded-full border-2 border-transparent border-t-primary animate-spin" />
    </div>
  );
}

function NavButtons({
  step,
  onBack,
  onNext,
  nextLabel = "Next",
  nextDisabled = false,
}: {
  step: number;
  onBack: () => void;
  onNext: () => void;
  nextLabel?: string;
  nextDisabled?: boolean;
}) {
  return (
    <div className="flex items-center justify-between mt-10">
      {step > 1 ? (
        <button
          onClick={onBack}
          className="px-5 py-2.5 rounded-xl text-sm font-medium text-muted-foreground hover:text-foreground hover:bg-white/5 transition-all"
        >
          Back
        </button>
      ) : (
        <div />
      )}
      <button
        onClick={onNext}
        disabled={nextDisabled}
        className="px-6 py-2.5 rounded-xl text-sm font-semibold bg-primary text-primary-foreground glow-primary hover:brightness-110 transition-all disabled:opacity-40 disabled:cursor-not-allowed"
      >
        {nextLabel}
      </button>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Step 1: Welcome                                                    */
/* ------------------------------------------------------------------ */

function StepWelcome({ onNext }: { onNext: () => void }) {
  return (
    <div className="flex flex-col items-center justify-center text-center animate-in fade-in slide-in-from-bottom-4 duration-500">
      {/* Logo */}
      <div className="w-24 h-24 rounded-3xl bg-gradient-to-br from-primary via-purple-500 to-indigo-400 flex items-center justify-center mb-8 shadow-[0_0_60px_rgba(99,102,241,0.3),0_0_120px_rgba(99,102,241,0.1)] animate-pulse-slow">
        <span className="text-3xl font-black text-white tracking-tighter select-none">
          Ax
        </span>
      </div>

      <h1 className="text-4xl font-bold text-foreground tracking-tight mb-3 text-glow">
        Welcome to AxiOS
      </h1>
      <p className="text-lg text-muted-foreground mb-2">
        Your AI-native operating system
      </p>
      <p className="text-sm text-muted-foreground/70 mb-10 max-w-md">
        Let&apos;s get you set up in a few minutes. We&apos;ll detect your
        hardware, configure AI backends, and personalize your experience.
      </p>

      <button
        onClick={onNext}
        className="px-8 py-3.5 rounded-2xl text-base font-semibold bg-primary text-primary-foreground shadow-[0_0_30px_rgba(99,102,241,0.35)] hover:shadow-[0_0_40px_rgba(99,102,241,0.5)] hover:brightness-110 transition-all duration-300"
      >
        Get Started
      </button>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Step 2: Hardware Detection                                         */
/* ------------------------------------------------------------------ */

function StepHardware({
  stats,
  loading,
  onBack,
  onNext,
}: {
  stats: SystemStats | null;
  loading: boolean;
  onBack: () => void;
  onNext: () => void;
}) {
  if (loading || !stats) {
    return (
      <div className="flex flex-col items-center justify-center animate-in fade-in duration-300">
        <Spinner className="w-10 h-10 mb-6" />
        <p className="text-sm text-muted-foreground">Detecting hardware...</p>
      </div>
    );
  }

  const primaryIp =
    stats.network?.interfaces?.find(
      (i) => i.status === "up" && i.ip && !i.ip.startsWith("127.")
    )?.ip ?? "N/A";

  const totalDisk = stats.disk?.reduce((a, d) => a + d.total_bytes, 0) ?? 0;
  const availDisk =
    stats.disk?.reduce((a, d) => a + d.available_bytes, 0) ?? 0;

  const cards = [
    {
      label: "CPU",
      icon: (
        <svg
          width="20"
          height="20"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <rect x="4" y="4" width="16" height="16" rx="2" />
          <rect x="9" y="9" width="6" height="6" />
          <path d="M9 1v3M15 1v3M9 20v3M15 20v3M20 9h3M20 14h3M1 9h3M1 14h3" />
        </svg>
      ),
      value: stats.cpu.model,
      sub: `${stats.cpu.cores} cores / ${stats.cpu.threads} threads`,
    },
    {
      label: "Memory",
      icon: (
        <svg
          width="20"
          height="20"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <rect x="2" y="6" width="20" height="12" rx="2" />
          <path d="M6 6V4M10 6V4M14 6V4M18 6V4M6 18v2M10 18v2M14 18v2M18 18v2" />
        </svg>
      ),
      value: formatBytes(stats.memory.total_bytes),
      sub: `${formatBytes(stats.memory.available_bytes)} available`,
    },
    {
      label: "Disk",
      icon: (
        <svg
          width="20"
          height="20"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <ellipse cx="12" cy="5" rx="9" ry="3" />
          <path d="M21 12c0 1.66-4 3-9 3s-9-1.34-9-3" />
          <path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5" />
        </svg>
      ),
      value: formatBytes(totalDisk),
      sub: `${formatBytes(availDisk)} available`,
    },
    {
      label: "Network",
      icon: (
        <svg
          width="20"
          height="20"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <path d="M5 12.55a11 11 0 0114.08 0M1.42 9a16 16 0 0121.16 0M8.53 16.11a6 6 0 016.95 0M12 20h.01" />
        </svg>
      ),
      value: primaryIp,
      sub: stats.network?.hostname ?? stats.hostname,
    },
  ];

  return (
    <div className="animate-in fade-in slide-in-from-right-4 duration-500">
      <h2 className="text-2xl font-bold mb-2">Hardware Detection</h2>
      <p className="text-sm text-muted-foreground mb-8">
        We detected the following hardware on your machine.
      </p>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        {cards.map((c) => (
          <div
            key={c.label}
            className="glass rounded-xl p-5 flex items-start gap-4"
          >
            <div className="w-10 h-10 rounded-lg bg-primary/10 flex items-center justify-center text-primary shrink-0">
              {c.icon}
            </div>
            <div className="min-w-0">
              <div className="flex items-center gap-2 mb-1">
                <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                  {c.label}
                </span>
                <svg
                  width="14"
                  height="14"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  className="text-emerald-400"
                >
                  <path d="M20 6L9 17l-5-5" />
                </svg>
              </div>
              <p className="text-sm font-medium text-foreground truncate">
                {c.value}
              </p>
              <p className="text-xs text-muted-foreground mt-0.5">{c.sub}</p>
            </div>
          </div>
        ))}
      </div>

      <div className="mt-6 flex items-center gap-2 text-emerald-400 text-sm font-medium">
        <svg
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
        >
          <path d="M20 6L9 17l-5-5" />
        </svg>
        Hardware detected successfully
      </div>

      <NavButtons step={2} onBack={onBack} onNext={onNext} />
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Step 3: AI Configuration                                           */
/* ------------------------------------------------------------------ */

interface OllamaStatus {
  running: boolean;
  models: string[];
}

function StepAI({
  stats,
  onBack,
  onNext,
}: {
  stats: SystemStats | null;
  onBack: () => void;
  onNext: () => void;
}) {
  // Cloud state
  const [provider, setProvider] = useState("anthropic");
  const [apiKey, setApiKey] = useState("");
  const [showKey, setShowKey] = useState(false);
  const [cloudConnecting, setCloudConnecting] = useState(false);
  const [cloudStatus, setCloudStatus] = useState<
    "idle" | "success" | "error"
  >("idle");
  const [cloudError, setCloudError] = useState("");

  // Local state
  const [ollamaStatus, setOllamaStatus] = useState<OllamaStatus>({
    running: false,
    models: [],
  });
  const [ollamaLoading, setOllamaLoading] = useState(true);
  const [pulling, setPulling] = useState(false);
  const [pullModel, setPullModel] = useState("");

  const recommendation = stats
    ? getModelRecommendation(stats.memory.total_bytes)
    : "qwen2.5:7b";

  // Check Ollama on mount
  useEffect(() => {
    (async () => {
      setOllamaLoading(true);
      try {
        const res = await fetch("/api/hosts");
        if (res.ok) {
          const data = await res.json();
          const hosts: Array<{
            name: string;
            url: string;
            healthy: boolean;
            models?: string[];
          }> = data.hosts ?? [];
          const local = hosts.find(
            (h) => h.name === "local" || h.url?.includes("localhost")
          );
          if (local && local.healthy) {
            // Fetch models
            const modelsRes = await fetch("/api/models/installed");
            let models: string[] = [];
            if (modelsRes.ok) {
              const md = await modelsRes.json();
              models = (md.models ?? []).map(
                (m: { name: string }) => m.name
              );
            }
            setOllamaStatus({ running: true, models });
          } else {
            setOllamaStatus({ running: false, models: [] });
          }
        }
      } catch {
        setOllamaStatus({ running: false, models: [] });
      } finally {
        setOllamaLoading(false);
      }
    })();
  }, []);

  const connectCloud = useCallback(async () => {
    setCloudConnecting(true);
    setCloudStatus("idle");
    setCloudError("");
    try {
      const res = await fetch("/api/providers/key", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ provider, key: apiKey }),
      });
      if (res.ok) {
        setCloudStatus("success");
      } else {
        const data = await res.json().catch(() => ({ error: "Connection failed" }));
        setCloudStatus("error");
        setCloudError(data.error || "Connection failed");
      }
    } catch {
      setCloudStatus("error");
      setCloudError("Network error");
    } finally {
      setCloudConnecting(false);
    }
  }, [provider, apiKey]);

  const handlePullModel = useCallback(async () => {
    const model = pullModel || recommendation;
    setPulling(true);
    try {
      await fetch("/api/models/pull", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: model }),
      });
      // Refresh model list
      const modelsRes = await fetch("/api/models/installed");
      if (modelsRes.ok) {
        const md = await modelsRes.json();
        const models = (md.models ?? []).map(
          (m: { name: string }) => m.name
        );
        setOllamaStatus((prev) => ({ ...prev, models }));
      }
    } catch {
      // silently fail
    } finally {
      setPulling(false);
    }
  }, [pullModel, recommendation]);

  const hasAI =
    cloudStatus === "success" ||
    (ollamaStatus.running && ollamaStatus.models.length > 0);

  return (
    <div className="animate-in fade-in slide-in-from-right-4 duration-500">
      <h2 className="text-2xl font-bold mb-2">AI Configuration</h2>
      <p className="text-sm text-muted-foreground mb-8">
        Connect at least one AI backend to power AxiOS intelligence.
      </p>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Cloud AI */}
        <div className="glass rounded-xl p-6">
          <h3 className="text-base font-semibold mb-1 flex items-center gap-2">
            <svg
              width="18"
              height="18"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
              className="text-primary"
            >
              <path d="M18 10h-1.26A8 8 0 109 20h9a5 5 0 000-10z" />
            </svg>
            Cloud AI
            <span className="text-xs text-muted-foreground font-normal">
              (optional)
            </span>
          </h3>
          <p className="text-xs text-muted-foreground mb-4">
            Connect a cloud AI provider for powerful models.
          </p>

          {cloudStatus === "success" ? (
            <div className="flex items-center gap-2 p-3 rounded-lg bg-emerald-500/10 text-emerald-400 text-sm font-medium">
              <svg
                width="16"
                height="16"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
              >
                <path d="M20 6L9 17l-5-5" />
              </svg>
              {CLOUD_PROVIDERS.find((p) => p.value === provider)?.label}{" "}
              connected
            </div>
          ) : (
            <>
              <label className="block text-xs text-muted-foreground mb-1.5">
                Provider
              </label>
              <select
                value={provider}
                onChange={(e) => {
                  setProvider(e.target.value);
                  setCloudStatus("idle");
                }}
                className="w-full mb-4 px-3 py-2 rounded-lg bg-white/5 border border-border text-sm text-foreground focus:outline-none focus:border-primary/50 transition-colors"
              >
                {CLOUD_PROVIDERS.map((p) => (
                  <option key={p.value} value={p.value}>
                    {p.label}
                  </option>
                ))}
              </select>

              <label className="block text-xs text-muted-foreground mb-1.5">
                API Key
              </label>
              <div className="relative mb-4">
                <input
                  type={showKey ? "text" : "password"}
                  value={apiKey}
                  onChange={(e) => setApiKey(e.target.value)}
                  placeholder="sk-..."
                  className="w-full px-3 py-2 pr-10 rounded-lg bg-white/5 border border-border text-sm text-foreground placeholder:text-muted-foreground/50 focus:outline-none focus:border-primary/50 transition-colors font-mono"
                />
                <button
                  type="button"
                  onClick={() => setShowKey(!showKey)}
                  className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-muted-foreground hover:text-foreground transition-colors"
                >
                  {showKey ? (
                    <svg
                      width="16"
                      height="16"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="1.5"
                    >
                      <path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19m-6.72-1.07a3 3 0 11-4.24-4.24" />
                      <line x1="1" y1="1" x2="23" y2="23" />
                    </svg>
                  ) : (
                    <svg
                      width="16"
                      height="16"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="1.5"
                    >
                      <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
                      <circle cx="12" cy="12" r="3" />
                    </svg>
                  )}
                </button>
              </div>

              {cloudStatus === "error" && (
                <p className="text-xs text-destructive mb-3">{cloudError}</p>
              )}

              <button
                onClick={connectCloud}
                disabled={!apiKey.trim() || cloudConnecting}
                className="w-full px-4 py-2 rounded-lg text-sm font-medium bg-primary text-primary-foreground hover:brightness-110 transition-all disabled:opacity-40 disabled:cursor-not-allowed flex items-center justify-center gap-2"
              >
                {cloudConnecting && <Spinner className="w-4 h-4" />}
                {cloudConnecting ? "Connecting..." : "Connect"}
              </button>
            </>
          )}
        </div>

        {/* Local AI */}
        <div className="glass rounded-xl p-6">
          <h3 className="text-base font-semibold mb-1 flex items-center gap-2">
            <svg
              width="18"
              height="18"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
              className="text-primary"
            >
              <rect x="4" y="4" width="16" height="16" rx="2" />
              <rect x="9" y="9" width="6" height="6" />
              <path d="M9 1v3M15 1v3M9 20v3M15 20v3M20 9h3M20 14h3M1 9h3M1 14h3" />
            </svg>
            Local AI
            <span className="text-xs text-muted-foreground font-normal">
              (optional)
            </span>
          </h3>
          <p className="text-xs text-muted-foreground mb-4">
            Run models locally via Ollama for privacy and speed.
          </p>

          {ollamaLoading ? (
            <div className="flex items-center gap-3 text-sm text-muted-foreground">
              <Spinner className="w-4 h-4" />
              Checking for Ollama...
            </div>
          ) : ollamaStatus.running ? (
            <div>
              <div className="flex items-center gap-2 text-emerald-400 text-sm font-medium mb-4">
                <svg
                  width="16"
                  height="16"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                >
                  <path d="M20 6L9 17l-5-5" />
                </svg>
                Ollama detected
              </div>

              {ollamaStatus.models.length > 0 ? (
                <div>
                  <p className="text-xs text-muted-foreground mb-2">
                    Installed models:
                  </p>
                  <div className="flex flex-wrap gap-2">
                    {ollamaStatus.models.map((m) => (
                      <span
                        key={m}
                        className="px-2.5 py-1 rounded-md bg-white/5 border border-border text-xs font-mono text-foreground"
                      >
                        {m}
                      </span>
                    ))}
                  </div>
                </div>
              ) : (
                <div>
                  <p className="text-xs text-muted-foreground mb-3">
                    No models installed. Based on your RAM (
                    {stats
                      ? formatBytes(stats.memory.total_bytes)
                      : "unknown"}
                    ), we recommend:
                  </p>
                  <div className="flex items-center gap-2 mb-4">
                    <span className="px-2.5 py-1 rounded-md bg-primary/10 border border-primary/20 text-xs font-mono text-primary">
                      {recommendation}
                    </span>
                  </div>
                  <div className="flex items-center gap-2">
                    <input
                      type="text"
                      value={pullModel}
                      onChange={(e) => setPullModel(e.target.value)}
                      placeholder={recommendation}
                      className="flex-1 px-3 py-2 rounded-lg bg-white/5 border border-border text-sm text-foreground placeholder:text-muted-foreground/50 focus:outline-none focus:border-primary/50 transition-colors font-mono"
                    />
                    <button
                      onClick={handlePullModel}
                      disabled={pulling}
                      className="px-4 py-2 rounded-lg text-sm font-medium bg-primary text-primary-foreground hover:brightness-110 transition-all disabled:opacity-40 disabled:cursor-not-allowed flex items-center gap-2 shrink-0"
                    >
                      {pulling && <Spinner className="w-4 h-4" />}
                      {pulling ? "Pulling..." : "Pull Model"}
                    </button>
                  </div>
                </div>
              )}
            </div>
          ) : (
            <div>
              <div className="flex items-center gap-2 text-yellow-400 text-sm font-medium mb-3">
                <svg
                  width="16"
                  height="16"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                >
                  <circle cx="12" cy="12" r="10" />
                  <line x1="12" y1="8" x2="12" y2="12" />
                  <line x1="12" y1="16" x2="12.01" y2="16" />
                </svg>
                Ollama not detected
              </div>
              <p className="text-xs text-muted-foreground mb-2">
                Install Ollama to run AI models locally:
              </p>
              <pre className="px-3 py-2 rounded-lg bg-black/30 border border-border text-xs font-mono text-foreground/80 select-all">
                curl -fsSL https://ollama.com/install.sh | sh
              </pre>
            </div>
          )}
        </div>
      </div>

      {!hasAI && (
        <p className="text-xs text-muted-foreground/60 text-center mt-6">
          Configure at least one AI backend to continue.
        </p>
      )}

      <NavButtons
        step={3}
        onBack={onBack}
        onNext={onNext}
        nextDisabled={!hasAI}
      />
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Step 4: Personalization                                            */
/* ------------------------------------------------------------------ */

function StepPersonalize({
  systemName,
  setSystemName,
  profiles,
  setProfiles,
  hostname,
  onBack,
  onNext,
}: {
  systemName: string;
  setSystemName: (v: string) => void;
  profiles: Profile[];
  setProfiles: (v: Profile[]) => void;
  hostname: string;
  onBack: () => void;
  onNext: () => void;
}) {
  const toggleProfile = (id: Profile) => {
    setProfiles(
      profiles.includes(id)
        ? profiles.filter((p) => p !== id)
        : [...profiles, id]
    );
  };

  return (
    <div className="animate-in fade-in slide-in-from-right-4 duration-500">
      <h2 className="text-2xl font-bold mb-2">Personalization</h2>
      <p className="text-sm text-muted-foreground mb-8">
        Give your machine a name and tell us how you plan to use it.
      </p>

      <label className="block text-xs text-muted-foreground mb-1.5 font-medium">
        System Name
      </label>
      <input
        type="text"
        value={systemName}
        onChange={(e) => setSystemName(e.target.value)}
        placeholder={hostname || "My AxiOS Machine"}
        className="w-full mb-8 px-4 py-2.5 rounded-xl bg-white/5 border border-border text-sm text-foreground placeholder:text-muted-foreground/50 focus:outline-none focus:border-primary/50 transition-colors"
      />

      <label className="block text-xs text-muted-foreground mb-3 font-medium">
        What will you use this machine for?
      </label>
      <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
        {PROFILES.map((p) => {
          const active = profiles.includes(p.id);
          return (
            <button
              key={p.id}
              onClick={() => toggleProfile(p.id)}
              className={`glass rounded-xl p-4 text-left transition-all duration-200 ${
                active
                  ? "border-primary/40 bg-primary/10 shadow-[0_0_15px_rgba(99,102,241,0.15)]"
                  : "hover:bg-white/5"
              }`}
            >
              <div className="text-2xl mb-2">{p.icon}</div>
              <div className="text-sm font-medium text-foreground">
                {p.title}
              </div>
              <div className="text-xs text-muted-foreground mt-0.5">
                {p.description}
              </div>
              {active && (
                <div className="mt-2">
                  <svg
                    width="14"
                    height="14"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="2.5"
                    className="text-primary"
                  >
                    <path d="M20 6L9 17l-5-5" />
                  </svg>
                </div>
              )}
            </button>
          );
        })}
      </div>

      <NavButtons step={4} onBack={onBack} onNext={onNext} />
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Step 5: Ready                                                      */
/* ------------------------------------------------------------------ */

function StepReady({
  systemName,
  profiles,
  cloudProvider,
  cloudConnected,
  ollamaModels,
  onComplete,
  completing,
  onBack,
}: {
  systemName: string;
  profiles: Profile[];
  cloudProvider: string;
  cloudConnected: boolean;
  ollamaModels: string[];
  onComplete: () => void;
  completing: boolean;
  onBack: () => void;
}) {
  const aiSummary = (() => {
    const parts: string[] = [];
    if (cloudConnected) {
      const label =
        CLOUD_PROVIDERS.find((p) => p.value === cloudProvider)?.label ??
        cloudProvider;
      parts.push(`Cloud: ${label}`);
    }
    if (ollamaModels.length > 0) {
      parts.push(`Local Ollama: ${ollamaModels.join(", ")}`);
    }
    return parts.length > 0 ? parts.join(" | ") : "None configured";
  })();

  const profileLabels = profiles
    .map((id) => PROFILES.find((p) => p.id === id)?.title)
    .filter(Boolean)
    .join(", ");

  return (
    <div className="flex flex-col items-center justify-center text-center animate-in fade-in slide-in-from-right-4 duration-500">
      {/* Animated checkmark */}
      <div className="w-24 h-24 rounded-full bg-emerald-500/10 flex items-center justify-center mb-8 shadow-[0_0_60px_rgba(52,211,153,0.2)]">
        <svg
          width="48"
          height="48"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          className="text-emerald-400"
        >
          <path d="M20 6L9 17l-5-5" className="animate-draw-check" />
        </svg>
      </div>

      <h2 className="text-3xl font-bold mb-3 text-glow">AxiOS is Ready</h2>
      <p className="text-sm text-muted-foreground mb-8 max-w-md">
        Your system is configured and ready to go. Here&apos;s a summary of
        your setup.
      </p>

      {/* Summary cards */}
      <div className="w-full max-w-md space-y-3 text-left mb-10">
        <div className="glass rounded-xl p-4 flex items-center gap-3">
          <div className="w-8 h-8 rounded-lg bg-primary/10 flex items-center justify-center text-primary shrink-0">
            <svg
              width="16"
              height="16"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
            >
              <path d="M21 16V8a2 2 0 00-1-1.73l-7-4a2 2 0 00-2 0l-7 4A2 2 0 003 8v8a2 2 0 001 1.73l7 4a2 2 0 002 0l7-4A2 2 0 0021 16z" />
            </svg>
          </div>
          <div className="min-w-0">
            <p className="text-xs text-muted-foreground">AI</p>
            <p className="text-sm font-medium text-foreground truncate">
              {aiSummary}
            </p>
          </div>
        </div>

        <div className="glass rounded-xl p-4 flex items-center gap-3">
          <div className="w-8 h-8 rounded-lg bg-primary/10 flex items-center justify-center text-primary shrink-0">
            <svg
              width="16"
              height="16"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
            >
              <rect x="2" y="2" width="20" height="8" rx="2" />
              <rect x="2" y="14" width="20" height="8" rx="2" />
              <circle cx="6" cy="6" r="1" />
              <circle cx="6" cy="18" r="1" />
            </svg>
          </div>
          <div className="min-w-0">
            <p className="text-xs text-muted-foreground">System</p>
            <p className="text-sm font-medium text-foreground truncate">
              {systemName || "AxiOS Machine"}
            </p>
          </div>
        </div>

        {profileLabels && (
          <div className="glass rounded-xl p-4 flex items-center gap-3">
            <div className="w-8 h-8 rounded-lg bg-primary/10 flex items-center justify-center text-primary shrink-0">
              <svg
                width="16"
                height="16"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.5"
              >
                <path d="M20 21v-2a4 4 0 00-4-4H8a4 4 0 00-4 4v2" />
                <circle cx="12" cy="7" r="4" />
              </svg>
            </div>
            <div className="min-w-0">
              <p className="text-xs text-muted-foreground">Profiles</p>
              <p className="text-sm font-medium text-foreground truncate">
                {profileLabels}
              </p>
            </div>
          </div>
        )}
      </div>

      <div className="flex items-center gap-4">
        <button
          onClick={onBack}
          className="px-5 py-2.5 rounded-xl text-sm font-medium text-muted-foreground hover:text-foreground hover:bg-white/5 transition-all"
        >
          Back
        </button>
        <button
          onClick={onComplete}
          disabled={completing}
          className="px-8 py-3.5 rounded-2xl text-base font-semibold bg-primary text-primary-foreground shadow-[0_0_30px_rgba(99,102,241,0.35)] hover:shadow-[0_0_40px_rgba(99,102,241,0.5)] hover:brightness-110 transition-all duration-300 disabled:opacity-50 flex items-center gap-2"
        >
          {completing && <Spinner className="w-4 h-4" />}
          Enter AxiOS
        </button>
      </div>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Main Wizard                                                        */
/* ------------------------------------------------------------------ */

export function SetupWizard({ onComplete }: SetupWizardProps) {
  const [step, setStep] = useState(1);

  // Hardware data (fetched once)
  const [stats, setStats] = useState<SystemStats | null>(null);
  const [statsLoading, setStatsLoading] = useState(false);
  const statsFetched = useRef(false);

  // Personalization
  const [systemName, setSystemName] = useState("");
  const [profiles, setProfiles] = useState<Profile[]>([]);

  // AI config tracking (for summary)
  const [cloudProvider, setCloudProvider] = useState("anthropic");
  const [cloudConnected, setCloudConnected] = useState(false);
  const [ollamaModels, setOllamaModels] = useState<string[]>([]);

  // Completion
  const [completing, setCompleting] = useState(false);

  // Fetch hardware when entering step 2
  useEffect(() => {
    if (step === 2 && !statsFetched.current) {
      statsFetched.current = true;
      setStatsLoading(true);
      fetch("/api/system/stats")
        .then((res) => (res.ok ? res.json() : null))
        .then((data) => {
          if (data) {
            setStats(data);
            setSystemName(data.hostname || "");
          }
        })
        .catch(() => {})
        .finally(() => setStatsLoading(false));
    }
  }, [step]);

  // When leaving step 3, capture AI state for the summary
  const captureAIState = useCallback(async () => {
    // Check cloud
    try {
      const res = await fetch("/api/providers");
      if (res.ok) {
        const data = await res.json();
        const providers: Array<{
          name: string;
          configured: boolean;
          active: boolean;
        }> = data.providers ?? [];
        const active = providers.find((p) => p.configured);
        if (active) {
          setCloudProvider(active.name.toLowerCase());
          setCloudConnected(true);
        }
      }
    } catch {
      // ignore
    }

    // Check local models
    try {
      const res = await fetch("/api/models/installed");
      if (res.ok) {
        const data = await res.json();
        const models = (data.models ?? []).map(
          (m: { name: string }) => m.name
        );
        setOllamaModels(models);
      }
    } catch {
      // ignore
    }
  }, []);

  const goNext = useCallback(async () => {
    if (step === 3) {
      await captureAIState();
    }
    setStep((s) => Math.min(s + 1, TOTAL_STEPS));
  }, [step, captureAIState]);

  const goBack = useCallback(() => {
    setStep((s) => Math.max(s - 1, 1));
  }, []);

  const handleComplete = useCallback(async () => {
    setCompleting(true);
    try {
      const config = {
        systemName: systemName || "AxiOS Machine",
        profiles,
        setupCompletedAt: new Date().toISOString(),
      };

      // Save to localStorage
      localStorage.setItem("axios-settings", JSON.stringify(config));

      // Mark complete on backend
      await fetch("/api/setup/complete", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(config),
      });

      onComplete();
    } catch {
      // Even if the API call fails, allow entering the app
      onComplete();
    } finally {
      setCompleting(false);
    }
  }, [systemName, profiles, onComplete]);

  return (
    <div className="fixed inset-0 z-50 flex flex-col items-center justify-center bg-background overflow-y-auto">
      {/* Animated background */}
      <div className="absolute inset-0 overflow-hidden pointer-events-none">
        <div className="absolute top-1/4 -left-32 w-96 h-96 bg-primary/5 rounded-full blur-[120px]" />
        <div className="absolute bottom-1/4 -right-32 w-96 h-96 bg-purple-500/5 rounded-full blur-[120px]" />
        <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[600px] h-[600px] bg-primary/[0.02] rounded-full blur-[100px]" />
      </div>

      {/* Progress indicator */}
      <div className="relative z-10 mb-10">
        <ProgressDots current={step} />
      </div>

      {/* Content */}
      <div className="relative z-10 w-full max-w-2xl px-6">
        {step === 1 && <StepWelcome onNext={goNext} />}
        {step === 2 && (
          <StepHardware
            stats={stats}
            loading={statsLoading}
            onBack={goBack}
            onNext={goNext}
          />
        )}
        {step === 3 && (
          <StepAI stats={stats} onBack={goBack} onNext={goNext} />
        )}
        {step === 4 && (
          <StepPersonalize
            systemName={systemName}
            setSystemName={setSystemName}
            profiles={profiles}
            setProfiles={setProfiles}
            hostname={stats?.hostname ?? ""}
            onBack={goBack}
            onNext={goNext}
          />
        )}
        {step === 5 && (
          <StepReady
            systemName={systemName}
            profiles={profiles}
            cloudProvider={cloudProvider}
            cloudConnected={cloudConnected}
            ollamaModels={ollamaModels}
            onComplete={handleComplete}
            completing={completing}
            onBack={goBack}
          />
        )}
      </div>
    </div>
  );
}
