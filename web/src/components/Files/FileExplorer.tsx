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
    year: "numeric",
    month: "short",
    day: "numeric",
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

  return (
    <div className="flex flex-col h-full bg-neutral-950">
      {/* Toolbar */}
      <div className="flex items-center gap-2 px-3 py-2 border-b border-neutral-800 bg-neutral-900/50">
        {/* Navigation buttons */}
        <button
          onClick={goUp}
          disabled={currentPath === "/"}
          className="p-1.5 rounded text-neutral-400 hover:text-neutral-100 hover:bg-neutral-800 disabled:opacity-30 disabled:hover:bg-transparent disabled:hover:text-neutral-400 transition-colors"
          title="Go up"
        >
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
            <path d="M8 12V4M4 7l4-4 4 4" />
          </svg>
        </button>

        <button
          onClick={refresh}
          className="p-1.5 rounded text-neutral-400 hover:text-neutral-100 hover:bg-neutral-800 transition-colors"
          title="Refresh"
        >
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
            <path d="M13 8a5 5 0 1 1-1-3M13 2v3h-3" />
          </svg>
        </button>

        {/* Breadcrumb */}
        <div className="flex-1 min-w-0">
          <Breadcrumb path={currentPath} onNavigate={navigateTo} />
        </div>

        {/* View toggle */}
        <div className="flex items-center border border-neutral-700 rounded overflow-hidden shrink-0">
          <button
            onClick={() => setViewMode("grid")}
            className={`p-1.5 transition-colors ${
              viewMode === "grid"
                ? "bg-neutral-700 text-neutral-100"
                : "text-neutral-500 hover:text-neutral-300"
            }`}
            title="Grid view"
          >
            <svg width="14" height="14" viewBox="0 0 14 14" fill="currentColor">
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
                ? "bg-neutral-700 text-neutral-100"
                : "text-neutral-500 hover:text-neutral-300"
            }`}
            title="List view"
          >
            <svg width="14" height="14" viewBox="0 0 14 14" fill="currentColor">
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
            <div className="flex items-center gap-3 text-neutral-500">
              <div className="w-4 h-4 border-2 border-neutral-600 border-t-blue-500 rounded-full animate-spin" />
              <span className="text-sm">Loading...</span>
            </div>
          </div>
        )}

        {error && (
          <div className="m-4 p-4 rounded-lg bg-red-950/30 border border-red-900/50">
            <p className="text-red-400 text-sm">{error}</p>
            <button
              onClick={refresh}
              className="mt-2 text-xs text-red-300 hover:text-red-100 underline"
            >
              Retry
            </button>
          </div>
        )}

        {!loading && !error && entries.length === 0 && (
          <div className="flex items-center justify-center h-32 text-neutral-600 text-sm">
            Empty directory
          </div>
        )}

        {!loading && !error && entries.length > 0 && viewMode === "grid" && (
          <div className="p-3 grid grid-cols-[repeat(auto-fill,minmax(120px,1fr))] gap-2">
            {entries.map((entry) => (
              <button
                key={entry.name}
                onClick={() => handleEntryClick(entry)}
                className="flex flex-col items-center gap-1.5 p-3 rounded-lg hover:bg-neutral-800/70 transition-colors group text-center"
              >
                <FileIcon
                  name={entry.name}
                  isDir={entry.type === "dir"}
                  className="text-2xl group-hover:scale-110 transition-transform"
                />
                <span className="text-xs text-neutral-300 truncate w-full leading-tight">
                  {entry.name}
                </span>
                {entry.type === "file" && (
                  <span className="text-[10px] font-mono text-neutral-600">
                    {formatSize(entry.size)}
                  </span>
                )}
              </button>
            ))}
          </div>
        )}

        {!loading && !error && entries.length > 0 && viewMode === "list" && (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-neutral-500 text-xs border-b border-neutral-800 sticky top-0 bg-neutral-950">
                <th className="py-2 px-4 font-medium">
                  <button
                    onClick={() => setSort("name")}
                    className="hover:text-neutral-300 transition-colors"
                  >
                    Name
                    <SortIndicator active={sortKey === "name"} dir={sortDir} />
                  </button>
                </th>
                <th className="py-2 px-4 font-medium text-right">
                  <button
                    onClick={() => setSort("size")}
                    className="hover:text-neutral-300 transition-colors"
                  >
                    Size
                    <SortIndicator active={sortKey === "size"} dir={sortDir} />
                  </button>
                </th>
                <th className="py-2 px-4 font-medium">
                  <button
                    onClick={() => setSort("mod_time")}
                    className="hover:text-neutral-300 transition-colors"
                  >
                    Modified
                    <SortIndicator active={sortKey === "mod_time"} dir={sortDir} />
                  </button>
                </th>
                <th className="py-2 px-4 font-medium">Permissions</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((entry) => (
                <tr
                  key={entry.name}
                  onClick={() => handleEntryClick(entry)}
                  className="hover:bg-neutral-800/50 cursor-pointer border-b border-neutral-800/30 transition-colors"
                >
                  <td className="py-1.5 px-4">
                    <div className="flex items-center gap-2">
                      <FileIcon
                        name={entry.name}
                        isDir={entry.type === "dir"}
                        className="text-base"
                      />
                      <span className="text-neutral-200 truncate">{entry.name}</span>
                    </div>
                  </td>
                  <td className="py-1.5 px-4 text-right font-mono text-neutral-500 text-xs">
                    {entry.type === "dir" ? "-" : formatSize(entry.size)}
                  </td>
                  <td className="py-1.5 px-4 text-neutral-500 text-xs">
                    {formatDate(entry.mod_time)}
                  </td>
                  <td className="py-1.5 px-4 font-mono text-neutral-600 text-xs">
                    {entry.permissions ?? "-"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Status bar */}
      <div className="flex items-center justify-between px-3 py-1.5 border-t border-neutral-800 text-xs text-neutral-600 bg-neutral-900/30">
        <span>{entries.length} items</span>
        <span className="font-mono">{currentPath}</span>
      </div>

      {/* File preview modal */}
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
