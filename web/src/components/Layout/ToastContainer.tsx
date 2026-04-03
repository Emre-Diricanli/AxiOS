import { useEffect, useState } from "react";
import { useToasts, dismissToast } from "@/hooks/useToast";
import type { Toast } from "@/hooks/useToast";

/* ── Icons ──────────────────────────────────────────────── */

function SuccessIcon() {
  return (
    <svg
      width="18"
      height="18"
      viewBox="0 0 24 24"
      fill="none"
      className="shrink-0"
    >
      <circle cx="12" cy="12" r="10" fill="rgba(34,197,94,0.15)" stroke="#22c55e" strokeWidth="1.5" />
      <path d="M8 12.5l2.5 2.5L16 9.5" stroke="#22c55e" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function ErrorIcon() {
  return (
    <svg
      width="18"
      height="18"
      viewBox="0 0 24 24"
      fill="none"
      className="shrink-0"
    >
      <circle cx="12" cy="12" r="10" fill="rgba(239,68,68,0.15)" stroke="#ef4444" strokeWidth="1.5" />
      <path d="M15 9l-6 6M9 9l6 6" stroke="#ef4444" strokeWidth="2" strokeLinecap="round" />
    </svg>
  );
}

function WarningIcon() {
  return (
    <svg
      width="18"
      height="18"
      viewBox="0 0 24 24"
      fill="none"
      className="shrink-0"
    >
      <path
        d="M12 3L2 21h20L12 3z"
        fill="rgba(245,158,11,0.15)"
        stroke="#f59e0b"
        strokeWidth="1.5"
        strokeLinejoin="round"
      />
      <path d="M12 10v4" stroke="#f59e0b" strokeWidth="2" strokeLinecap="round" />
      <circle cx="12" cy="17" r="1" fill="#f59e0b" />
    </svg>
  );
}

function InfoIcon() {
  return (
    <svg
      width="18"
      height="18"
      viewBox="0 0 24 24"
      fill="none"
      className="shrink-0"
    >
      <circle cx="12" cy="12" r="10" fill="rgba(59,130,246,0.15)" stroke="#3b82f6" strokeWidth="1.5" />
      <path d="M12 16v-4" stroke="#3b82f6" strokeWidth="2" strokeLinecap="round" />
      <circle cx="12" cy="8" r="1" fill="#3b82f6" />
    </svg>
  );
}

function TypeIcon({ type }: { type: Toast["type"] }) {
  switch (type) {
    case "success":
      return <SuccessIcon />;
    case "error":
      return <ErrorIcon />;
    case "warning":
      return <WarningIcon />;
    case "info":
      return <InfoIcon />;
  }
}

const PROGRESS_COLORS: Record<Toast["type"], string> = {
  success: "bg-green-500",
  error: "bg-red-500",
  warning: "bg-amber-500",
  info: "bg-blue-500",
};

/* ── Single Toast ───────────────────────────────────────── */

function ToastItem({ toast: t }: { toast: Toast }) {
  const [visible, setVisible] = useState(false);
  const [progress, setProgress] = useState(100);

  // Slide-in animation
  useEffect(() => {
    const frame = requestAnimationFrame(() => setVisible(true));
    return () => cancelAnimationFrame(frame);
  }, []);

  // Progress bar countdown
  useEffect(() => {
    const interval = 50; // update every 50ms for smooth animation
    const timer = setInterval(() => {
      const elapsed = Date.now() - t.createdAt;
      const remaining = Math.max(0, 1 - elapsed / t.duration);
      setProgress(remaining * 100);
      if (remaining <= 0) {
        clearInterval(timer);
      }
    }, interval);
    return () => clearInterval(timer);
  }, [t.createdAt, t.duration]);

  return (
    <div
      className={`relative glass rounded-xl px-4 py-3 max-w-sm w-80 shadow-2xl overflow-hidden transition-all duration-300 ease-out ${
        visible
          ? "translate-x-0 opacity-100"
          : "translate-x-full opacity-0"
      }`}
    >
      {/* Content */}
      <div className="flex items-start gap-3">
        <div className="mt-0.5">
          <TypeIcon type={t.type} />
        </div>
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium text-foreground leading-tight">
            {t.title}
          </p>
          {t.message && (
            <p className="text-xs text-muted-foreground mt-0.5 leading-snug truncate">
              {t.message}
            </p>
          )}
        </div>
        <button
          onClick={() => dismissToast(t.id)}
          className="shrink-0 w-5 h-5 rounded-md flex items-center justify-center text-muted-foreground hover:text-foreground hover:bg-white/[0.08] transition-colors"
        >
          <svg
            width="12"
            height="12"
            viewBox="0 0 16 16"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            strokeLinecap="round"
          >
            <path d="M12 4L4 12M4 4l8 8" />
          </svg>
        </button>
      </div>

      {/* Progress bar */}
      <div className="absolute bottom-0 left-0 right-0 h-[2px] bg-white/[0.04]">
        <div
          className={`h-full ${PROGRESS_COLORS[t.type]} transition-[width] duration-100 ease-linear`}
          style={{ width: `${progress}%`, opacity: 0.6 }}
        />
      </div>
    </div>
  );
}

/* ── Container ──────────────────────────────────────────── */

export function ToastContainer() {
  const toasts = useToasts();

  if (toasts.length === 0) return null;

  return (
    <div className="fixed bottom-6 right-6 z-50 flex flex-col-reverse gap-2 pointer-events-none">
      {toasts.map((t) => (
        <div key={t.id} className="pointer-events-auto">
          <ToastItem toast={t} />
        </div>
      ))}
    </div>
  );
}
