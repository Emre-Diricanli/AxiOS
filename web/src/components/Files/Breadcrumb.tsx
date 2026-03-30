interface BreadcrumbProps {
  path: string;
  onNavigate: (path: string) => void;
}

export function Breadcrumb({ path, onNavigate }: BreadcrumbProps) {
  const segments = path.split("/").filter(Boolean);

  return (
    <nav className="flex items-center gap-1 text-sm font-mono min-w-0 overflow-x-auto">
      <button
        onClick={() => onNavigate("/")}
        className="shrink-0 px-1.5 py-0.5 rounded text-neutral-400 hover:text-neutral-100 hover:bg-neutral-800 transition-colors"
      >
        /
      </button>
      {segments.map((segment, i) => {
        const segmentPath = "/" + segments.slice(0, i + 1).join("/");
        const isLast = i === segments.length - 1;
        return (
          <span key={segmentPath} className="flex items-center gap-1 min-w-0">
            <span className="text-neutral-600 shrink-0">/</span>
            <button
              onClick={() => onNavigate(segmentPath)}
              className={`truncate px-1.5 py-0.5 rounded transition-colors ${
                isLast
                  ? "text-neutral-100 bg-neutral-800/50"
                  : "text-neutral-400 hover:text-neutral-100 hover:bg-neutral-800"
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
