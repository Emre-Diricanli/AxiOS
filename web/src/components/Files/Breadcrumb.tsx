import { useRef, useEffect } from "react";

interface BreadcrumbProps {
  path: string;
  onNavigate: (path: string) => void;
}

export function Breadcrumb({ path, onNavigate }: BreadcrumbProps) {
  const scrollRef = useRef<HTMLElement>(null);
  const segments = path.split("/").filter(Boolean);

  // Auto-scroll to the end when path changes
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollLeft = scrollRef.current.scrollWidth;
    }
  }, [path]);

  return (
    <nav
      ref={scrollRef}
      className="flex items-center gap-0.5 px-2 py-1 rounded-lg glass-subtle min-w-0 overflow-x-auto scrollbar-none"
    >
      <button
        onClick={() => onNavigate("/")}
        className={`shrink-0 px-2 py-0.5 rounded-md text-xs transition-colors ${
          segments.length === 0
            ? "text-foreground font-semibold bg-white/[0.06]"
            : "text-muted-foreground hover:text-foreground hover:bg-white/[0.06]"
        }`}
      >
        /
      </button>
      {segments.map((segment, i) => {
        const segmentPath = "/" + segments.slice(0, i + 1).join("/");
        const isLast = i === segments.length - 1;
        return (
          <span key={segmentPath} className="flex items-center gap-0.5 min-w-0 shrink-0">
            <svg
              width="12"
              height="12"
              viewBox="0 0 16 16"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
              className="text-muted-foreground/30 shrink-0"
            >
              <path d="M6 4l4 4-4 4" />
            </svg>
            <button
              onClick={() => onNavigate(segmentPath)}
              className={`truncate max-w-[140px] px-2 py-0.5 rounded-md text-xs transition-colors ${
                isLast
                  ? "text-foreground font-semibold bg-white/[0.06]"
                  : "text-muted-foreground hover:text-foreground hover:bg-white/[0.06]"
              }`}
            >
              {segment}
            </button>
          </span>
        );
      })}
    </nav>
  );
}
