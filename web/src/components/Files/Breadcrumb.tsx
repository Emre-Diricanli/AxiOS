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
        className="shrink-0 px-1.5 py-0.5 rounded-md text-neutral-500 hover:text-neutral-200 hover:bg-white/[0.06] transition-colors"
      >
        /
      </button>
      {segments.map((segment, i) => {
        const segmentPath = "/" + segments.slice(0, i + 1).join("/");
        const isLast = i === segments.length - 1;
        return (
          <span key={segmentPath} className="flex items-center gap-0.5 min-w-0">
            <span className="text-neutral-700 shrink-0">/</span>
            <button
              onClick={() => onNavigate(segmentPath)}
              className={`truncate max-w-[120px] px-1.5 py-0.5 rounded-md transition-colors ${
                isLast
                  ? "text-neutral-200 font-medium"
                  : "text-neutral-500 hover:text-neutral-200 hover:bg-white/[0.06]"
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
