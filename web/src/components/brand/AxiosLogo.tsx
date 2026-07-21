import { useId } from "react";
import { cn } from "@/lib/utils";

interface AxiosMarkProps {
  className?: string;
  /** When true, omits the glass app tile (symbol only). */
  bare?: boolean;
  title?: string;
}

/** Crosshair reticle + sharp open A — primary app mark. */
export function AxiosMark({ className, bare = false, title = "AxiOS" }: AxiosMarkProps) {
  const uid = useId().replace(/:/g, "");
  const tileId = `axios-tile-${uid}`;
  const strokeId = `axios-stroke-${uid}`;
  const coreId = `axios-core-${uid}`;
  const glowId = `axios-glow-${uid}`;

  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 128 128"
      className={cn("block", className)}
      role="img"
      aria-label={title}
    >
      <title>{title}</title>
      <defs>
        <linearGradient id={tileId} x1="18%" y1="0%" x2="82%" y2="100%">
          <stop offset="0%" stopColor="#1c2233" />
          <stop offset="55%" stopColor="#12151c" />
          <stop offset="100%" stopColor="#0b0d10" />
        </linearGradient>
        <linearGradient id={strokeId} x1="30%" y1="5%" x2="70%" y2="95%">
          <stop offset="0%" stopColor="#d4dbff" />
          <stop offset="40%" stopColor="#9aa8f8" />
          <stop offset="100%" stopColor="#6d7fe8" />
        </linearGradient>
        <radialGradient id={coreId} cx="50%" cy="48%" r="50%">
          <stop offset="0%" stopColor="#ffffff" />
          <stop offset="30%" stopColor="#eef1ff" />
          <stop offset="70%" stopColor="#a8b6ff" stopOpacity="0.55" />
          <stop offset="100%" stopColor="#7890f8" stopOpacity="0" />
        </radialGradient>
        <filter id={glowId} x="-60%" y="-60%" width="220%" height="220%">
          <feGaussianBlur stdDeviation="2" result="b" />
          <feMerge>
            <feMergeNode in="b" />
            <feMergeNode in="SourceGraphic" />
          </feMerge>
        </filter>
      </defs>

      {!bare && (
        <>
          <rect x="4" y="4" width="120" height="120" rx="28" fill={`url(#${tileId})`} />
          <rect
            x="5"
            y="5"
            width="118"
            height="118"
            rx="27"
            fill="none"
            stroke="#7890f8"
            strokeOpacity="0.22"
            strokeWidth="1.25"
          />
        </>
      )}

      <g transform="translate(64 64)" filter={`url(#${glowId})`}>
        <g stroke="#9aa8f8" strokeOpacity="0.5" strokeWidth="1.35" strokeLinecap="round">
          <line x1="0" y1="-44" x2="0" y2="-35" />
          <line x1="0" y1="35" x2="0" y2="44" />
          <line x1="-44" y1="0" x2="-35" y2="0" />
          <line x1="35" y1="0" x2="44" y2="0" />
          <line x1="-31.1" y1="-31.1" x2="-24.7" y2="-24.7" />
          <line x1="24.7" y1="-24.7" x2="31.1" y2="-31.1" />
          <line x1="-31.1" y1="31.1" x2="-24.7" y2="24.7" />
          <line x1="24.7" y1="24.7" x2="31.1" y2="31.1" />
        </g>
        <circle r="31" fill="none" stroke="#9aa8f8" strokeOpacity="0.42" strokeWidth="1.4" />
        <g stroke="#9aa8f8" strokeOpacity="0.28" strokeWidth="1">
          <line x1="0" y1="-31" x2="0" y2="31" />
          <line x1="-31" y1="0" x2="31" y2="0" />
        </g>
        <path
          d="M0 -27 L-23 29 M0 -27 L23 29 M-8.5 9.5 H8.5"
          fill="none"
          stroke={`url(#${strokeId})`}
          strokeWidth="4"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <circle r="8.5" fill={`url(#${coreId})`} />
        <circle r="2" fill="#ffffff" />
      </g>
    </svg>
  );
}

interface AxiosOrbitalMarkProps {
  className?: string;
  title?: string;
}

/** Compact orbital-A monogram from the preferred wordmark lockup. */
export function AxiosOrbitalMark({ className, title = "AxiOS" }: AxiosOrbitalMarkProps) {
  const uid = useId().replace(/:/g, "");
  const strokeId = `axios-orb-${uid}`;
  const glowId = `axios-orb-glow-${uid}`;

  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 96 96"
      className={cn("block", className)}
      role="img"
      aria-label={title}
    >
      <title>{title}</title>
      <defs>
        <linearGradient id={strokeId} x1="10%" y1="0%" x2="90%" y2="100%">
          <stop offset="0%" stopColor="#b7c3ff" />
          <stop offset="50%" stopColor="#7890f8" />
          <stop offset="100%" stopColor="#5a6fd0" />
        </linearGradient>
        <filter id={glowId} x="-40%" y="-40%" width="180%" height="180%">
          <feGaussianBlur stdDeviation="1.4" result="b" />
          <feMerge>
            <feMergeNode in="b" />
            <feMergeNode in="SourceGraphic" />
          </feMerge>
        </filter>
      </defs>
      <g transform="translate(8 6)" filter={`url(#${glowId})`}>
        <path
          d="M40 6 L12 84 H28 L33.5 68 H54.5 L60 84 H76 L48 6 Z M36.5 54 L44 30 L51.5 54 Z"
          fill={`url(#${strokeId})`}
          fillRule="evenodd"
        />
        <path
          d="M6 68 C 4 38, 22 4, 58 10 C 82 14, 90 40, 80 64"
          fill="none"
          stroke={`url(#${strokeId})`}
          strokeWidth="4"
          strokeLinecap="round"
        />
        <circle cx="72" cy="16" r="4.5" fill="#dce3ff" />
        <circle cx="72" cy="16" r="2.2" fill="#ffffff" />
      </g>
    </svg>
  );
}

interface AxiosLogoProps {
  className?: string;
  markClassName?: string;
  wordmarkClassName?: string;
  /** Hide the wordmark (icon-only). */
  markOnly?: boolean;
  /** Which monogram to use next to the wordmark. Default: orbital (wordmark set). */
  variant?: "orbital" | "crosshair";
}

/** Brand lockup — preferred orbital A + AxiOS wordmark by default. */
export function AxiosLogo({
  className,
  markClassName,
  wordmarkClassName,
  markOnly = false,
  variant = "orbital",
}: AxiosLogoProps) {
  return (
    <div className={cn("inline-flex items-center gap-2.5 min-w-0", className)}>
      {variant === "crosshair" ? (
        <AxiosMark className={cn("w-7 h-7 shrink-0", markClassName)} />
      ) : (
        <AxiosOrbitalMark className={cn("w-7 h-7 shrink-0", markClassName)} />
      )}
      {!markOnly && (
        <span
          className={cn(
            "text-sm font-semibold tracking-tight text-foreground truncate",
            wordmarkClassName
          )}
        >
          Axi<span className="text-primary">OS</span>
        </span>
      )}
    </div>
  );
}
