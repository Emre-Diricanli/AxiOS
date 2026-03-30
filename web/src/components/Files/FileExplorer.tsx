import { useState } from "react";
import { useFileSystem } from "@/hooks/useFileSystem";
import { Breadcrumb } from "./Breadcrumb";
import { FileIcon } from "./FileIcon";
import { FilePreview } from "./FilePreview";
import type { FileEntry } from "@/types/messages";

type ViewMode = "grid" | "list";

function formatSize(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const value = bytes / Math.pow(1024, i);
  return `${value.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

function formatDate(dateStr?: string): string {
  if (!dateStr) return "-";
  const d = new Date(dateStr);
  return d.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function SortIndicator({ active, dir }: { active: boolean; dir: "asc" | "desc" }) {
  if (!active) return null;
  return <span className="ml-1 text-blue-400">{dir === "asc" ? "\u25B2" : "\u25BC"}</span>;
}

export function FileExplorer() {
  const {
    currentPath,
    entries,
    loading,
    error,
    navigateTo,
    goUp,
    refresh,
    sortKey,
    sortDir,
    setSort,
  } = useFileSystem("/");

  const [viewMode, setViewMode] = useState<ViewMode>("grid");
  const [selectedFile, setSelectedFile] = useState<FileEntry | null>(null);

  const handleEntryClick = (entry: FileEntry) => {
    if (entry.type === "dir") {
      const next =
        currentPath === "/" ? `/${entry.name}` : `${currentPath}/${entry.name}`;
      navigateTo(next);
    } else {
      setSelectedFile(entry);
    }
  };

  const dirCount = entries.filter((e) => e.type === "dir").length;
  const fileCount = entries.filter((e) => e.type === "file").length;

  return (
    <div className="flex flex-col h-full bg-background">
      {/* Toolbar */}
      <div className="flex items-center gap-2 px-3 py-2 border-b border-border bg-card/60 shrink-0">
        <button
          onClick={goUp}
          disabled={currentPath === "/"}
          className="p-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted disabled:opacity-20 disabled:hover:bg-transparent transition-colors"
          title="Go up"
        >
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
            <path d="M8 12V4M4 7l4-4 4 4" />
          </svg>
        </button>

        <button
          onClick={refresh}
          className="p-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
          title="Refresh"
        >
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
            <path d="M13 8a5 5 0 1 1-1-3M13 2v3h-3" />
          </svg>
        </button>

        <div className="flex-1 min-w-0">
          <Breadcrumb path={currentPath} onNavigate={navigateTo} />
        </div>

        <div className="flex items-center border border-border rounded-md overflow-hidden shrink-0">
          <button
            onClick={() => setViewMode("grid")}
            className={`p-1.5 transition-colors ${
              viewMode === "grid"
                ? "bg-white/[0.08] text-foreground"
                : "text-muted-foreground hover:text-neutral-400"
            }`}
          >
            <svg width="12" height="12" viewBox="0 0 14 14" fill="currentColor">
              <rect x="0" y="0" width="6" height="6" rx="1" />
              <rect x="8" y="0" width="6" height="6" rx="1" />
              <rect x="0" y="8" width="6" height="6" rx="1" />
              <rect x="8" y="8" width="6" height="6" rx="1" />
            </svg>
          </button>
          <button
            onClick={() => setViewMode("list")}
            className={`p-1.5 transition-colors ${
              viewMode === "list"
                ? "bg-white/[0.08] text-foreground"
                : "text-muted-foreground hover:text-neutral-400"
            }`}
          >
            <svg width="12" height="12" viewBox="0 0 14 14" fill="currentColor">
              <rect x="0" y="1" width="14" height="2" rx="0.5" />
              <rect x="0" y="6" width="14" height="2" rx="0.5" />
              <rect x="0" y="11" width="14" height="2" rx="0.5" />
            </svg>
          </button>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto min-h-0">
        {loading && (
          <div className="flex items-center justify-center h-32">
            <div className="w-5 h-5 border-2 border-neutral-700 border-t-blue-500 rounded-full animate-spin" />
          </div>
        )}

        {error && (
          <div className="m-4 p-4 rounded-lg bg-red-500/5 border border-red-500/20">
            <p className="text-red-400 text-xs">{error}</p>
            <button onClick={refresh} className="mt-2 text-[10px] text-red-300 hover:text-red-100 underline">
              Retry
            </button>
          </div>
        )}

        {!loading && !error && entries.length === 0 && (
          <div className="flex items-center justify-center h-32 text-muted-foreground/50 text-xs">
            Empty directory
          </div>
        )}

        {!loading && !error && entries.length > 0 && viewMode === "grid" && (
          <div className="p-3 grid grid-cols-[repeat(auto-fill,minmax(110px,1fr))] gap-1">
            {entries.map((entry) => (
              <button
                key={entry.name}
                onClick={() => handleEntryClick(entry)}
                className="flex flex-col items-center gap-1 p-3 rounded-lg hover:bg-accent transition-all group text-center"
              >
                <FileIcon
                  name={entry.name}
                  isDir={entry.type === "dir"}
                  className="text-2xl group-hover:scale-105 transition-transform"
                />
                <span className="text-[11px] text-foreground/80 truncate w-full leading-tight">
                  {entry.name}
                </span>
                {entry.type === "file" && (
                  <span className="text-[9px] font-mono text-muted-foreground/50">
                    {formatSize(entry.size)}
                  </span>
                )}
              </button>
            ))}
          </div>
        )}

        {!loading && !error && entries.length > 0 && viewMode === "list" && (
          <table className="w-full text-xs">
            <thead>
              <tr className="text-left text-muted-foreground text-[10px] border-b border-border sticky top-0 bg-background">
                <th className="py-2 px-4 font-medium">
                  <button onClick={() => setSort("name")} className="hover:text-foreground/80 transition-colors uppercase tracking-wider">
                    Name <SortIndicator active={sortKey === "name"} dir={sortDir} />
                  </button>
                </th>
                <th className="py-2 px-4 font-medium text-right">
                  <button onClick={() => setSort("size")} className="hover:text-foreground/80 transition-colors uppercase tracking-wider">
                    Size <SortIndicator active={sortKey === "size"} dir={sortDir} />
                  </button>
                </th>
                <th className="py-2 px-4 font-medium">
                  <button onClick={() => setSort("mod_time")} className="hover:text-foreground/80 transition-colors uppercase tracking-wider">
                    Modified <SortIndicator active={sortKey === "mod_time"} dir={sortDir} />
                  </button>
                </th>
                <th className="py-2 px-4 font-medium uppercase tracking-wider">Permissions</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((entry) => (
                <tr
                  key={entry.name}
                  onClick={() => handleEntryClick(entry)}
                  className="hover:bg-accent/50 cursor-pointer border-b border-white/[0.03] transition-colors"
                >
                  <td className="py-1.5 px-4">
                    <div className="flex items-center gap-2">
                      <FileIcon name={entry.name} isDir={entry.type === "dir"} className="text-sm" />
                      <span className="text-foreground/80 truncate">{entry.name}</span>
                    </div>
                  </td>
                  <td className="py-1.5 px-4 text-right font-mono text-muted-foreground">
                    {entry.type === "dir" ? "-" : formatSize(entry.size)}
                  </td>
                  <td className="py-1.5 px-4 text-muted-foreground">{formatDate(entry.mod_time)}</td>
                  <td className="py-1.5 px-4 font-mono text-muted-foreground/50">{entry.permissions ?? "-"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Status bar */}
      <div className="flex items-center justify-between px-3 py-1.5 border-t border-border text-[10px] text-muted-foreground bg-card/40 shrink-0">
        <span>{dirCount} folders, {fileCount} files</span>
        <span className="font-mono text-muted-foreground/50">{currentPath}</span>
      </div>

      {selectedFile && (
        <FilePreview
          file={selectedFile}
          currentPath={currentPath}
          onClose={() => setSelectedFile(null)}
        />
      )}
    </div>
  );
}
