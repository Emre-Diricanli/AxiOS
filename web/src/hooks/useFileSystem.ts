import { useState, useCallback, useEffect } from "react";
import type { FileEntry } from "@/types/messages";

type SortKey = "name" | "size" | "mod_time";
type SortDir = "asc" | "desc";

interface UseFileSystemReturn {
  currentPath: string;
  entries: FileEntry[];
  loading: boolean;
  error: string | null;
  navigateTo: (path: string) => void;
  goUp: () => void;
  goToRoot: () => void;
  refresh: () => void;
  sortKey: SortKey;
  sortDir: SortDir;
  setSort: (key: SortKey) => void;
  showDotfiles: boolean;
  setShowDotfiles: (show: boolean) => void;
}

function sortEntries(entries: FileEntry[], key: SortKey, dir: SortDir): FileEntry[] {
  const sorted = [...entries];
  sorted.sort((a, b) => {
    // Directories always first
    if (a.type !== b.type) {
      return a.type === "dir" ? -1 : 1;
    }

    let cmp = 0;
    switch (key) {
      case "name":
        cmp = a.name.localeCompare(b.name, undefined, { sensitivity: "base" });
        break;
      case "size":
        cmp = a.size - b.size;
        break;
      case "mod_time":
        cmp = (a.mod_time ?? "").localeCompare(b.mod_time ?? "");
        break;
    }

    return dir === "asc" ? cmp : -cmp;
  });
  return sorted;
}

export function useFileSystem(initialPath = "/"): UseFileSystemReturn {
  const [currentPath, setCurrentPath] = useState(initialPath);
  const [rawEntries, setRawEntries] = useState<FileEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sortKey, setSortKey] = useState<SortKey>("name");
  const [sortDir, setSortDir] = useState<SortDir>("asc");
  const [showDotfiles, setShowDotfiles] = useState(false);

  const fetchDirectory = useCallback((path: string) => {
    setLoading(true);
    setError(null);

    fetch(`/api/fs/list?path=${encodeURIComponent(path)}`)
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return res.json();
      })
      .then((data: { entries: FileEntry[] }) => {
        setRawEntries(data.entries ?? []);
        setCurrentPath(path);
      })
      .catch((err: Error) => {
        setError(err.message);
        setRawEntries([]);
      })
      .finally(() => setLoading(false));
  }, []);

  // Fetch on mount
  useEffect(() => {
    fetchDirectory(initialPath);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const navigateTo = useCallback(
    (path: string) => {
      const normalized = path === "" ? "/" : path;
      fetchDirectory(normalized);
    },
    [fetchDirectory]
  );

  const goUp = useCallback(() => {
    if (currentPath === "/") return;
    const parent = currentPath.split("/").slice(0, -1).join("/") || "/";
    fetchDirectory(parent);
  }, [currentPath, fetchDirectory]);

  const goToRoot = useCallback(() => {
    fetchDirectory("/");
  }, [fetchDirectory]);

  const refresh = useCallback(() => {
    fetchDirectory(currentPath);
  }, [currentPath, fetchDirectory]);

  const setSort = useCallback(
    (key: SortKey) => {
      if (key === sortKey) {
        setSortDir((d) => (d === "asc" ? "desc" : "asc"));
      } else {
        setSortKey(key);
        setSortDir("asc");
      }
    },
    [sortKey]
  );

  const filtered = showDotfiles
    ? rawEntries
    : rawEntries.filter((e) => !e.name.startsWith("."));
  const entries = sortEntries(filtered, sortKey, sortDir);

  return {
    currentPath,
    entries,
    loading,
    error,
    navigateTo,
    goUp,
    goToRoot,
    refresh,
    sortKey,
    sortDir,
    setSort,
    showDotfiles,
    setShowDotfiles,
  };
}
