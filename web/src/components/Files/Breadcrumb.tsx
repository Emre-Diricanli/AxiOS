interface BreadcrumbProps {
  path: string;
  onNavigate: (path: string) => void;
}

export function Breadcrumb({ path, onNavigate }: BreadcrumbProps) {
  const segments = path.split("/").filter(Boolean);

  return (
    <nav className="flex items-center gap-0.5 text-xs font-mono min-w-0 overflow-x-auto scrollbar-none">
      <button
        onClick={() => onNavigate("/")}
        className="shrink-0 px-1.5 py-0.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
      >
        /
      </button>
      {segments.map((segment, i) => {
        const segmentPath = "/" + segments.slice(0, i + 1).join("/");
        const isLast = i === segments.length - 1;
        return (
          <span key={segmentPath} className="flex items-center gap-0.5 min-w-0">
            <span className="text-muted-foreground/40 shrink-0">/</span>
            <button
              onClick={() => onNavigate(segmentPath)}
              className={`truncate max-w-[120px] px-1.5 py-0.5 rounded-md transition-colors ${
                isLast
                  ? "text-foreground font-medium"
                  : "text-muted-foreground hover:text-foreground hover:bg-accent"
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
