import { useState, useCallback, useMemo, useRef } from "react";
import { useFileSystem } from "@/hooks/useFileSystem";
import { Breadcrumb } from "./Breadcrumb";
import { FileIcon } from "./FileIcon";
import { FilePreview } from "./FilePreview";
import type { FileEntry } from "@/types/messages";

type ViewMode = "grid" | "list";

/* ---------- Helpers ---------- */

function formatSize(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const value = bytes / Math.pow(1024, i);
  return `${value.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

function formatDate(dateStr?: string): string {
  if (!dateStr) return "--";
  const d = new Date(dateStr);
  return d.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function getFileKind(entry: FileEntry): string {
  if (entry.type === "dir") return "Folder";
  if (!entry.name.includes(".")) return "File";
  const ext = entry.name.split(".").pop()?.toLowerCase() ?? "";
  const map: Record<string, string> = {
    ts: "TypeScript", tsx: "TypeScript", js: "JavaScript", jsx: "JavaScript",
    py: "Python", go: "Go", rs: "Rust", java: "Java", c: "C Source",
    cpp: "C++ Source", h: "Header", cs: "C#", rb: "Ruby", php: "PHP",
    swift: "Swift", kt: "Kotlin", sh: "Shell Script",
    md: "Markdown", txt: "Text", pdf: "PDF Document",
    json: "JSON", yaml: "YAML", yml: "YAML", toml: "TOML", xml: "XML",
    png: "PNG Image", jpg: "JPEG Image", jpeg: "JPEG Image", gif: "GIF Image",
    svg: "SVG Image", webp: "WebP Image",
    mp4: "MP4 Video", mov: "QuickTime Movie", mkv: "MKV Video",
    mp3: "MP3 Audio", wav: "WAV Audio", flac: "FLAC Audio",
    zip: "ZIP Archive", tar: "TAR Archive", gz: "GZip Archive",
    css: "CSS", scss: "SCSS", html: "HTML",
  };
  return map[ext] ?? `${ext.toUpperCase()} File`;
}

/* ---------- Sidebar Items ---------- */

interface SidebarItem {
  id: string;
  label: string;
  path: string;
  icon: "home" | "desktop" | "documents" | "downloads" | "root" | "drive";
}

const FAVORITES: SidebarItem[] = [
  { id: "home", label: "Home", path: "~", icon: "home" },
  { id: "desktop", label: "Desktop", path: "~/Desktop", icon: "desktop" },
  { id: "documents", label: "Documents", path: "~/Documents", icon: "documents" },
  { id: "downloads", label: "Downloads", path: "~/Downloads", icon: "downloads" },
  { id: "root", label: "Root", path: "/", icon: "root" },
];

const LOCATIONS: SidebarItem[] = [
  { id: "disk", label: "Macintosh HD", path: "/", icon: "drive" },
];

function SidebarIcon({ type, active }: { type: SidebarItem["icon"]; active: boolean }) {
  const color = active ? "text-primary" : "text-muted-foreground";
  const size = 15;

  switch (type) {
    case "home":
      return (
        <svg width={size} height={size} viewBox="0 0 16 16" fill="none" className={color}>
          <path d="M2 8.5l6-6 6 6" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
          <path d="M3.5 7.5v5.5a1 1 0 001 1h7a1 1 0 001-1V7.5" stroke="currentColor" strokeWidth="1.3" />
          <path d="M6 14V10.5a1 1 0 011-1h2a1 1 0 011 1V14" stroke="currentColor" strokeWidth="1.3" />
        </svg>
      );
    case "desktop":
      return (
        <svg width={size} height={size} viewBox="0 0 16 16" fill="none" className={color}>
          <rect x="1.5" y="2.5" width="13" height="9" rx="1" stroke="currentColor" strokeWidth="1.3" />
          <path d="M5.5 14h5M8 11.5v2.5" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
        </svg>
      );
    case "documents":
      return (
        <svg width={size} height={size} viewBox="0 0 16 16" fill="none" className={color}>
          <path d="M4 1.5h5.5L13 5v9a1.5 1.5 0 01-1.5 1.5h-7A1.5 1.5 0 013 14V3a1.5 1.5 0 011-1.5z" stroke="currentColor" strokeWidth="1.3" />
          <path d="M9 1.5V5.5h4" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
          <path d="M5.5 8.5h5M5.5 11h3" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
        </svg>
      );
    case "downloads":
      return (
        <svg width={size} height={size} viewBox="0 0 16 16" fill="none" className={color}>
          <path d="M8 2v8M5 7.5L8 10.5 11 7.5" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
          <path d="M2.5 11v2a1.5 1.5 0 001.5 1.5h8a1.5 1.5 0 001.5-1.5v-2" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
        </svg>
      );
    case "root":
      return (
        <svg width={size} height={size} viewBox="0 0 16 16" fill="none" className={color}>
          <path d="M4 7L8 3l4 4" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
          <path d="M4 12L8 8l4 4" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      );
    case "drive":
      return (
        <svg width={size} height={size} viewBox="0 0 16 16" fill="none" className={color}>
          <rect x="1.5" y="5" width="13" height="6" rx="1.5" stroke="currentColor" strokeWidth="1.3" />
          <circle cx="11.5" cy="8" r="1" fill="currentColor" />
          <path d="M3.5 8h4" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
        </svg>
      );
  }
}

/* ---------- Sort indicator ---------- */

function SortIndicator({ active, dir }: { active: boolean; dir: "asc" | "desc" }) {
  if (!active) return <span className="ml-1 opacity-0 text-[8px]">^</span>;
  return (
    <svg
      width="10"
      height="10"
      viewBox="0 0 10 10"
      fill="currentColor"
      className={`ml-1 text-primary inline-block transition-transform ${dir === "desc" ? "rotate-180" : ""}`}
    >
      <path d="M2 7L5 3l3 4H2z" />
    </svg>
  );
}

/* ---------- FileExplorer ---------- */

export function FileExplorer() {
  const {
    currentPath,
    entries,
    loading,
    error,
    navigateTo,
    refresh,
    sortKey,
    sortDir,
    setSort,
    showDotfiles,
    setShowDotfiles,
  } = useFileSystem("/");

  const [viewMode, setViewMode] = useState<ViewMode>("grid");
  const [selectedFile, setSelectedFile] = useState<FileEntry | null>(null);
  const [searchQuery, setSearchQuery] = useState("");
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [showPreview, setShowPreview] = useState(false);

  // Navigation history
  const [history, setHistory] = useState<string[]>(["/"]);
  const [historyIndex, setHistoryIndex] = useState(0);
  const suppressHistoryRef = useRef(false);

  const handleNavigate = useCallback(
    (path: string) => {
      const normalized = path === "" ? "/" : path;
      navigateTo(normalized);
      setSelectedFile(null);
      setSearchQuery("");

      if (suppressHistoryRef.current) {
        suppressHistoryRef.current = false;
        return;
      }
      setHistory((prev) => {
        const sliced = prev.slice(0, historyIndex + 1);
        return [...sliced, normalized];
      });
      setHistoryIndex((prev) => prev + 1);
    },
    [navigateTo, historyIndex],
  );

  const canGoBack = historyIndex > 0;
  const canGoForward = historyIndex < history.length - 1;

  const goBack = useCallback(() => {
    if (!canGoBack) return;
    const newIdx = historyIndex - 1;
    setHistoryIndex(newIdx);
    suppressHistoryRef.current = true;
    navigateTo(history[newIdx]);
    setSelectedFile(null);
    setSearchQuery("");
  }, [canGoBack, historyIndex, history, navigateTo]);

  const goForward = useCallback(() => {
    if (!canGoForward) return;
    const newIdx = historyIndex + 1;
    setHistoryIndex(newIdx);
    suppressHistoryRef.current = true;
    navigateTo(history[newIdx]);
    setSelectedFile(null);
    setSearchQuery("");
  }, [canGoForward, historyIndex, history, navigateTo]);

  // Sidebar navigation (resolve ~ paths)
  const handleSidebarNav = useCallback(
    (path: string) => {
      // Pass ~ paths directly — the backend expands them to the actual home dir
      handleNavigate(path);
    },
    [handleNavigate],
  );

  // Filter entries by search
  const filteredEntries = useMemo(() => {
    if (!searchQuery.trim()) return entries;
    const q = searchQuery.toLowerCase();
    return entries.filter((e) => e.name.toLowerCase().includes(q));
  }, [entries, searchQuery]);

  // Selection
  const handleSelect = useCallback((entry: FileEntry) => {
    setSelectedFile(entry);
    setShowPreview(true);
  }, []);

  const handleOpen = useCallback(
    (entry: FileEntry) => {
      if (entry.type === "dir") {
        const next =
          currentPath === "/" ? `/${entry.name}` : `${currentPath}/${entry.name}`;
        handleNavigate(next);
      } else {
        setSelectedFile(entry);
        setShowPreview(true);
      }
    },
    [currentPath, handleNavigate],
  );

  // Stats
  const dirCount = filteredEntries.filter((e) => e.type === "dir").length;
  const fileCount = filteredEntries.filter((e) => e.type === "file").length;
  const selectedCount = selectedFile ? 1 : 0;

  // Check if a sidebar item matches current path
  const isSidebarActive = (item: SidebarItem) => {
    if (item.path === "/") return currentPath === "/";
    if (item.path === "~") return currentPath.endsWith(currentPath.split("/").pop() ?? "") && currentPath.includes("/Users/") || currentPath.includes("/home/") || currentPath.includes("/root");
    // For ~/Desktop etc, check if current path ends with the subfolder
    const sub = item.path.replace("~/", "");
    return currentPath.endsWith("/" + sub) && (currentPath.includes("/Users/") || currentPath.includes("/home/") || currentPath.includes("/root"));
  };

  return (
    <div className="flex h-full overflow-hidden">
      {/* ===== Sidebar ===== */}
      {sidebarOpen && (
        <div className="w-[200px] shrink-0 glass-subtle border-r border-border flex flex-col overflow-y-auto scrollbar-none">
          {/* Favorites */}
          <div className="px-3 pt-3 pb-1">
            <p className="text-[10px] font-semibold text-muted-foreground/60 uppercase tracking-widest mb-1">
              Favorites
            </p>
          </div>
          <div className="px-1.5 space-y-0.5">
            {FAVORITES.map((item) => {
              const active = isSidebarActive(item);
              return (
                <button
                  key={item.id}
                  onClick={() => handleSidebarNav(item.path)}
                  className={`w-full flex items-center gap-2.5 px-2.5 py-1.5 rounded-lg text-xs transition-all duration-150 ${
                    active
                      ? "bg-accent text-foreground font-medium"
                      : "text-muted-foreground hover:text-foreground hover:bg-white/[0.04]"
                  }`}
                >
                  <SidebarIcon type={item.icon} active={active} />
                  <span className="truncate">{item.label}</span>
                </button>
              );
            })}
          </div>

          {/* Locations */}
          <div className="px-3 pt-4 pb-1">
            <p className="text-[10px] font-semibold text-muted-foreground/60 uppercase tracking-widest mb-1">
              Locations
            </p>
          </div>
          <div className="px-1.5 space-y-0.5">
            {LOCATIONS.map((item) => {
              const active = isSidebarActive(item);
              return (
                <button
                  key={item.id}
                  onClick={() => handleSidebarNav(item.path)}
                  className={`w-full flex items-center gap-2.5 px-2.5 py-1.5 rounded-lg text-xs transition-all duration-150 ${
                    active
                      ? "bg-accent text-foreground font-medium"
                      : "text-muted-foreground hover:text-foreground hover:bg-white/[0.04]"
                  }`}
                >
                  <SidebarIcon type={item.icon} active={active} />
                  <span className="truncate">{item.label}</span>
                </button>
              );
            })}
          </div>

          <div className="flex-1" />
        </div>
      )}

      {/* ===== Main area ===== */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* Toolbar */}
        <div className="flex items-center gap-1.5 px-2 h-10 border-b border-border shrink-0">
          {/* Sidebar toggle */}
          <button
            onClick={() => setSidebarOpen(!sidebarOpen)}
            className="p-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-white/[0.06] transition-colors"
            title={sidebarOpen ? "Hide sidebar" : "Show sidebar"}
          >
            <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.3">
              <rect x="1.5" y="2.5" width="13" height="11" rx="1.5" />
              <path d="M6 2.5v11" />
            </svg>
          </button>

          {/* Separator */}
          <div className="w-px h-4 bg-border" />

          {/* Back / Forward */}
          <button
            onClick={goBack}
            disabled={!canGoBack}
            className="p-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-white/[0.06] disabled:opacity-20 disabled:hover:bg-transparent transition-colors"
            title="Back"
          >
            <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
              <path d="M10 3L5 8l5 5" />
            </svg>
          </button>
          <button
            onClick={goForward}
            disabled={!canGoForward}
            className="p-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-white/[0.06] disabled:opacity-20 disabled:hover:bg-transparent transition-colors"
            title="Forward"
          >
            <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
              <path d="M6 3l5 5-5 5" />
            </svg>
          </button>

          {/* Breadcrumb */}
          <div className="flex-1 min-w-0 mx-1">
            <Breadcrumb path={currentPath} onNavigate={handleNavigate} />
          </div>

          {/* Search */}
          <div className="relative shrink-0">
            <svg
              width="12"
              height="12"
              viewBox="0 0 16 16"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
              className="absolute left-2 top-1/2 -translate-y-1/2 text-muted-foreground/50"
            >
              <circle cx="7" cy="7" r="4.5" />
              <path d="M10.5 10.5L14 14" />
            </svg>
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search"
              className="w-[120px] focus:w-[160px] h-7 pl-7 pr-2 text-xs rounded-md bg-white/[0.04] border border-border text-foreground placeholder:text-muted-foreground/40 focus:outline-none focus:border-primary/40 transition-all duration-200"
            />
          </div>

          {/* Dotfiles toggle */}
          <button
            onClick={() => setShowDotfiles(!showDotfiles)}
            title={showDotfiles ? "Hide dotfiles" : "Show dotfiles"}
            className={`px-1.5 py-1 rounded-md text-[10px] font-mono font-bold transition-colors shrink-0 ${
              showDotfiles
                ? "bg-primary/15 text-primary border-glow"
                : "text-muted-foreground hover:text-foreground hover:bg-white/[0.06]"
            }`}
          >
            .*
          </button>

          {/* View mode */}
          <div className="flex items-center border border-border rounded-md overflow-hidden shrink-0">
            <button
              onClick={() => setViewMode("grid")}
              className={`p-1.5 transition-colors ${
                viewMode === "grid"
                  ? "bg-white/[0.08] text-foreground"
                  : "text-muted-foreground hover:text-foreground"
              }`}
              title="Grid view"
            >
              <svg width="12" height="12" viewBox="0 0 14 14" fill="currentColor">
                <rect x="0" y="0" width="6" height="6" rx="1.5" />
                <rect x="8" y="0" width="6" height="6" rx="1.5" />
                <rect x="0" y="8" width="6" height="6" rx="1.5" />
                <rect x="8" y="8" width="6" height="6" rx="1.5" />
              </svg>
            </button>
            <button
              onClick={() => setViewMode("list")}
              className={`p-1.5 transition-colors ${
                viewMode === "list"
                  ? "bg-white/[0.08] text-foreground"
                  : "text-muted-foreground hover:text-foreground"
              }`}
              title="List view"
            >
              <svg width="12" height="12" viewBox="0 0 14 14" fill="currentColor">
                <rect x="0" y="1" width="14" height="2" rx="0.5" />
                <rect x="0" y="6" width="14" height="2" rx="0.5" />
                <rect x="0" y="11" width="14" height="2" rx="0.5" />
              </svg>
            </button>
          </div>

          {/* Refresh */}
          <button
            onClick={refresh}
            className="p-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-white/[0.06] transition-colors"
            title="Refresh"
          >
            <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
              <path d="M13 8a5 5 0 1 1-1-3M13 2v3h-3" />
            </svg>
          </button>
        </div>

        {/* Content + Preview wrapper */}
        <div className="flex-1 flex min-h-0">
          {/* Content area */}
          <div className="flex-1 overflow-y-auto scrollbar-none min-h-0 min-w-0">
            {/* Loading */}
            {loading && (
              <div className="flex items-center justify-center h-32">
                <div className="w-5 h-5 border-2 border-border border-t-primary rounded-full animate-spin" />
              </div>
            )}

            {/* Error */}
            {error && (
              <div className="m-4 p-4 rounded-xl bg-destructive/5 border border-destructive/20">
                <p className="text-destructive text-xs">{error}</p>
                <button
                  onClick={refresh}
                  className="mt-2 text-[10px] text-destructive/80 hover:text-destructive underline"
                >
                  Retry
                </button>
              </div>
            )}

            {/* Empty */}
            {!loading && !error && filteredEntries.length === 0 && (
              <div className="flex flex-col items-center justify-center h-32 text-muted-foreground/40">
                <svg width="24" height="24" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1" className="mb-2">
                  <path d="M2 4l2-2h4l1 1h5a1 1 0 011 1v8a1 1 0 01-1 1H2a1 1 0 01-1-1V4z" />
                </svg>
                <span className="text-xs">
                  {searchQuery ? "No matches found" : "Empty directory"}
                </span>
              </div>
            )}

            {/* Grid view */}
            {!loading && !error && filteredEntries.length > 0 && viewMode === "grid" && (
              <div className="p-3 grid grid-cols-[repeat(auto-fill,minmax(120px,1fr))] gap-1.5">
                {filteredEntries.map((entry) => {
                  const isSelected = selectedFile?.name === entry.name;
                  return (
                    <button
                      key={entry.name}
                      onClick={() => handleSelect(entry)}
                      onDoubleClick={() => handleOpen(entry)}
                      className={`flex flex-col items-center gap-1.5 p-3 rounded-xl transition-all duration-150 group text-center ${
                        isSelected
                          ? "border-glow bg-accent/50"
                          : "border border-transparent hover:bg-secondary hover:scale-[1.02]"
                      }`}
                    >
                      <div className="transition-transform duration-150 group-hover:scale-105">
                        <FileIcon
                          name={entry.name}
                          isDir={entry.type === "dir"}
                          size="lg"
                        />
                      </div>
                      <span className="text-[11px] text-foreground/80 truncate w-full leading-tight mt-0.5">
                        {entry.name}
                      </span>
                      {entry.type === "file" && (
                        <span className="text-[9px] font-mono text-muted-foreground/40">
                          {formatSize(entry.size)}
                        </span>
                      )}
                    </button>
                  );
                })}
              </div>
            )}

            {/* List view */}
            {!loading && !error && filteredEntries.length > 0 && viewMode === "list" && (
              <table className="w-full text-xs">
                <thead>
                  <tr className="text-left text-[10px] text-muted-foreground border-b border-border sticky top-0 bg-background/80 backdrop-blur-sm z-10">
                    <th className="py-2 px-4 font-medium">
                      <button
                        onClick={() => setSort("name")}
                        className="hover:text-foreground transition-colors uppercase tracking-wider flex items-center"
                      >
                        Name
                        <SortIndicator active={sortKey === "name"} dir={sortDir} />
                      </button>
                    </th>
                    <th className="py-2 px-4 font-medium">
                      <button
                        onClick={() => setSort("mod_time")}
                        className="hover:text-foreground transition-colors uppercase tracking-wider flex items-center"
                      >
                        Date Modified
                        <SortIndicator active={sortKey === "mod_time"} dir={sortDir} />
                      </button>
                    </th>
                    <th className="py-2 px-4 font-medium text-right">
                      <button
                        onClick={() => setSort("size")}
                        className="hover:text-foreground transition-colors uppercase tracking-wider flex items-center justify-end"
                      >
                        Size
                        <SortIndicator active={sortKey === "size"} dir={sortDir} />
                      </button>
                    </th>
                    <th className="py-2 px-4 font-medium uppercase tracking-wider">Kind</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredEntries.map((entry, idx) => {
                    const isSelected = selectedFile?.name === entry.name;
                    const isEven = idx % 2 === 0;
                    return (
                      <tr
                        key={entry.name}
                        onClick={() => handleSelect(entry)}
                        onDoubleClick={() => handleOpen(entry)}
                        className={`cursor-pointer transition-colors duration-100 ${
                          isSelected
                            ? "bg-accent border-l-2 border-l-primary"
                            : isEven
                              ? "hover:bg-white/[0.03] border-l-2 border-l-transparent"
                              : "bg-white/[0.01] hover:bg-white/[0.04] border-l-2 border-l-transparent"
                        }`}
                      >
                        <td className="py-1.5 px-4">
                          <div className="flex items-center gap-2.5">
                            <FileIcon name={entry.name} isDir={entry.type === "dir"} size="sm" />
                            <span className={`truncate ${isSelected ? "text-foreground font-medium" : "text-foreground/80"}`}>
                              {entry.name}
                            </span>
                          </div>
                        </td>
                        <td className="py-1.5 px-4 text-muted-foreground">{formatDate(entry.mod_time)}</td>
                        <td className="py-1.5 px-4 text-right font-mono text-muted-foreground">
                          {entry.type === "dir" ? "--" : formatSize(entry.size)}
                        </td>
                        <td className="py-1.5 px-4 text-muted-foreground">{getFileKind(entry)}</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            )}
          </div>

          {/* Preview panel */}
          {showPreview && selectedFile && (
            <FilePreview
              file={selectedFile}
              currentPath={currentPath}
              onClose={() => {
                setShowPreview(false);
                setSelectedFile(null);
              }}
            />
          )}
        </div>

        {/* Status bar */}
        <div className="flex items-center justify-between px-3 py-1.5 border-t border-border text-[10px] text-muted-foreground shrink-0">
          <span>
            {filteredEntries.length} item{filteredEntries.length !== 1 ? "s" : ""}
            {dirCount > 0 && ` (${dirCount} folder${dirCount !== 1 ? "s" : ""})`}
            {fileCount > 0 && ` (${fileCount} file${fileCount !== 1 ? "s" : ""})`}
            {selectedCount > 0 && `, ${selectedCount} selected`}
          </span>
          <div className="flex items-center gap-3">
            <span className="font-mono text-muted-foreground/50">{currentPath}</span>
            <span className="text-muted-foreground/30">|</span>
            <span className="text-muted-foreground/40">Local Storage</span>
          </div>
        </div>
      </div>
    </div>
  );
}
