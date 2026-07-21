import { useEffect, useId, useMemo, useState } from "react";
import { areaPath, smoothLinePath, type ChartPoint } from "./path";

export interface SeriesPoint {
  t: number; // epoch ms
  value: number; // percent
}

interface AreaChartProps {
  samples: SeriesPoint[];
  color: string; // stroke/fill base color
  height?: number; // rendered px height; width follows container
  label?: string; // y-unit suffix for the tooltip, defaults to "%"
  windowMs?: number; // visible time window, defaults to 60s
}

const W = 600;
const H = 220;
const PAD = { top: 14, right: 10, bottom: 6, left: 10 };

// Nice Y ceilings: the smallest step that fits the visible data with ~15%
// headroom. Keeps low-usage lines readable instead of squashed at the bottom.
const Y_STEPS = [1, 2, 5, 10, 15, 20, 30, 40, 50, 60, 70, 80, 90, 100];

function yCeiling(maxValue: number): number {
  const withHeadroom = maxValue * 1.15;
  for (const step of Y_STEPS) {
    if (withHeadroom <= step) return step;
  }
  return 100;
}

function formatAgo(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000));
  if (s < 5) return "now";
  if (s < 60) return `${s}s ago`;
  const m = Math.round(s / 60);
  return `${m}m ago`;
}

// useNow re-renders the chart ~30x/sec so the line glides continuously
// between polls. requestAnimationFrame pauses in background tabs for free.
function useNow(): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    let raf = 0;
    let last = 0;
    const loop = (t: number) => {
      if (t - last >= 33) {
        last = t;
        setNow(Date.now());
      }
      raf = requestAnimationFrame(loop);
    };
    raf = requestAnimationFrame(loop);
    return () => cancelAnimationFrame(raf);
  }, []);
  return now;
}

// AreaChart renders a rolling time series as a smoothed, gradient-filled SVG.
// X is mapped by timestamp against a fixed trailing window and redrawn on an
// animation frame loop, so the line slides smoothly while new samples still
// arrive at the (slow) poll rate — visual smoothness costs no extra network
// traffic. The Y domain auto-scales to the visible data (0 baseline, nice
// ceiling); axis labels live in HTML so the stretched SVG never distorts text.
export function AreaChart({ samples, color, height = 110, label = "%", windowMs = 60_000 }: AreaChartProps) {
  const gradientId = useId();
  const [hover, setHover] = useState<number | null>(null); // visible-sample index
  const now = useNow();

  const { line, area, points, yMax, visible } = useMemo(() => {
    const innerW = W - PAD.left - PAD.right;
    const innerH = H - PAD.top - PAD.bottom;

    // Trailing window ending at render-now; keep one sample before the left
    // edge so the line enters the frame smoothly.
    const leftEdge = now - windowMs;
    let start = samples.findIndex((s) => s.t >= leftEdge);
    if (start === -1) start = samples.length - 1;
    start = Math.max(0, start - 1);
    let vis = samples.slice(start);

    // Extend the leading edge flat to "now" so the line always reaches the
    // right edge instead of hanging a poll-interval behind the frame.
    if (vis.length > 0 && now - vis[vis.length - 1].t > 250) {
      vis = [...vis, { t: now, value: vis[vis.length - 1].value }];
    }

    const maxValue = vis.reduce((max, s) => Math.max(max, s.value), 0);
    const ceiling = yCeiling(maxValue);

    const pts: ChartPoint[] = vis.map((s) => ({
      x: PAD.left + (1 - (now - s.t) / windowMs) * innerW,
      y: PAD.top + (1 - Math.min(ceiling, Math.max(0, s.value)) / ceiling) * innerH,
    }));

    return {
      line: smoothLinePath(pts),
      area: areaPath(pts, H - PAD.bottom),
      points: pts,
      yMax: ceiling,
      visible: vis,
    };
  }, [samples, now, windowMs]);

  const handleMove = (event: React.MouseEvent<SVGRectElement>) => {
    if (points.length === 0) return;
    const rect = (event.currentTarget.ownerSVGElement as SVGSVGElement).getBoundingClientRect();
    const x = ((event.clientX - rect.left) / rect.width) * W;
    // Nearest sample by x coordinate.
    let best = 0;
    let bestDist = Infinity;
    for (let i = 0; i < points.length; i++) {
      const dist = Math.abs(points[i].x - x);
      if (dist < bestDist) {
        bestDist = dist;
        best = i;
      }
    }
    setHover(best);
  };

  const hoverSample = hover !== null ? visible[hover] : null;
  const hoverPoint = hover !== null ? points[hover] : null;
  const lastPoint = points.length > 0 ? points[points.length - 1] : null;

  if (samples.length < 2) {
    return (
      <div className="relative w-full flex items-center justify-center text-xs text-muted-foreground" style={{ height }}>
        Collecting samples…
      </div>
    );
  }

  return (
    <div className="relative w-full">
      {/* Y-scale marker */}
      <span className="absolute top-0 left-0 text-[9px] font-mono text-muted-foreground/60 select-none pointer-events-none">
        {yMax}{label}
      </span>

      <svg
        viewBox={`0 0 ${W} ${H}`}
        className="w-full block"
        style={{ height }}
        preserveAspectRatio="none"
        onMouseLeave={() => setHover(null)}
      >
        <defs>
          <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={color} stopOpacity="0.32" />
            <stop offset="100%" stopColor={color} stopOpacity="0.02" />
          </linearGradient>
        </defs>

        {/* Gridlines at thirds of the Y domain */}
        {[1 / 3, 2 / 3].map((frac) => {
          const y = PAD.top + (1 - frac) * (H - PAD.top - PAD.bottom);
          return (
            <line
              key={frac}
              x1={PAD.left}
              x2={W - PAD.right}
              y1={y}
              y2={y}
              stroke="var(--color-border)"
              strokeOpacity="0.5"
              strokeDasharray="3 5"
              strokeWidth="1"
              vectorEffect="non-scaling-stroke"
            />
          );
        })}

        {/* Baseline */}
        <line
          x1={PAD.left}
          x2={W - PAD.right}
          y1={H - PAD.bottom}
          y2={H - PAD.bottom}
          stroke="var(--color-border)"
          strokeOpacity="0.8"
          strokeWidth="1"
          vectorEffect="non-scaling-stroke"
        />

        <path d={area} fill={`url(#${gradientId})`} />
        <path d={line} fill="none" stroke={color} strokeWidth="1.75" vectorEffect="non-scaling-stroke" strokeLinejoin="round" strokeLinecap="round" />

        {/* Live endpoint */}
        {lastPoint && (
          <circle cx={lastPoint.x} cy={lastPoint.y} r="3" fill={color}>
            <animate attributeName="opacity" values="1;0.35;1" dur="2s" repeatCount="indefinite" />
          </circle>
        )}

        {/* Hover crosshair */}
        {hoverPoint && (
          <g pointerEvents="none">
            <line
              x1={hoverPoint.x}
              x2={hoverPoint.x}
              y1={PAD.top}
              y2={H - PAD.bottom}
              stroke={color}
              strokeOpacity="0.5"
              strokeWidth="1"
              vectorEffect="non-scaling-stroke"
            />
            <circle cx={hoverPoint.x} cy={hoverPoint.y} r="3.5" fill={color} stroke="var(--color-card)" strokeWidth="1.5" />
          </g>
        )}

        {/* Hover capture overlay */}
        <rect
          x={PAD.left}
          y={PAD.top}
          width={W - PAD.left - PAD.right}
          height={H - PAD.top - PAD.bottom}
          fill="transparent"
          onMouseMove={handleMove}
        />
      </svg>

      {/* X-axis labels (HTML so text stays crisp while the SVG stretches) */}
      <div className="flex justify-between mt-1 text-[10px] font-mono text-muted-foreground/70 select-none">
        <span>{formatAgo(windowMs)}</span>
        <span>{formatAgo(windowMs / 2)}</span>
        <span>now</span>
      </div>

      {/* Tooltip */}
      {hoverSample && hoverPoint && (
        <div
          className="absolute pointer-events-none glass-subtle rounded-md px-2 py-1 text-[11px] font-mono whitespace-nowrap z-10"
          style={{
            left: `${(hoverPoint.x / W) * 100}%`,
            top: 0,
            transform: `translateX(${hoverPoint.x / W > 0.75 ? "-110%" : "10px"})`,
          }}
        >
          <span className="text-foreground">{hoverSample.value.toFixed(1)}{label}</span>
          <span className="text-muted-foreground ml-2">{formatAgo(Date.now() - hoverSample.t)}</span>
        </div>
      )}
    </div>
  );
}
