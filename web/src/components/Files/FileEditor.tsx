import { useEffect, useRef, useState, useCallback } from "react";
import { EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter } from "@codemirror/view";
import { EditorState } from "@codemirror/state";
import { javascript } from "@codemirror/lang-javascript";
import { python } from "@codemirror/lang-python";
import { json } from "@codemirror/lang-json";
import { html } from "@codemirror/lang-html";
import { css } from "@codemirror/lang-css";
import { markdown } from "@codemirror/lang-markdown";
import { bracketMatching, indentOnInput } from "@codemirror/language";
import { oneDark } from "@codemirror/theme-one-dark";
import { FileIcon } from "./FileIcon";
import { toastSuccess, toastError } from "@/hooks/useToast";

/* ---------- Language detection ---------- */

interface LangInfo {
  label: string;
  extension: () => ReturnType<typeof javascript>;
}

const LANG_MAP: Record<string, LangInfo> = {
  js: { label: "JavaScript", extension: () => javascript() },
  jsx: { label: "JSX", extension: () => javascript({ jsx: true }) },
  ts: { label: "TypeScript", extension: () => javascript({ typescript: true }) },
  tsx: { label: "TSX", extension: () => javascript({ jsx: true, typescript: true }) },
  py: { label: "Python", extension: python },
  json: { label: "JSON", extension: json },
  html: { label: "HTML", extension: html },
  htm: { label: "HTML", extension: html },
  css: { label: "CSS", extension: css },
  scss: { label: "SCSS", extension: css },
  md: { label: "Markdown", extension: markdown },
  mdx: { label: "MDX", extension: markdown },
};

// Extensions that are editable but don't have specific language support
const PLAIN_EXTENSIONS: Record<string, string> = {
  txt: "Plain Text",
  log: "Log",
  go: "Go",
  rs: "Rust",
  java: "Java",
  c: "C",
  cpp: "C++",
  h: "Header",
  cs: "C#",
  rb: "Ruby",
  php: "PHP",
  swift: "Swift",
  kt: "Kotlin",
  sh: "Shell",
  bash: "Bash",
  zsh: "Zsh",
  yaml: "YAML",
  yml: "YAML",
  toml: "TOML",
  xml: "XML",
  sql: "SQL",
  dockerfile: "Dockerfile",
  makefile: "Makefile",
  gitignore: "Gitignore",
  env: "Environment",
  cfg: "Config",
  conf: "Config",
  ini: "INI",
  csv: "CSV",
  svg: "SVG",
  vue: "Vue",
  svelte: "Svelte",
  r: "R",
  lua: "Lua",
  mod: "Go Module",
  sum: "Go Sum",
  lock: "Lockfile",
};

function getLangInfo(filename: string): { label: string; extensions: any[] } {
  const ext = filename.includes(".")
    ? filename.split(".").pop()?.toLowerCase() ?? ""
    : filename.toLowerCase();

  const lang = LANG_MAP[ext];
  if (lang) {
    return { label: lang.label, extensions: [lang.extension()] };
  }

  const plainLabel = PLAIN_EXTENSIONS[ext];
  if (plainLabel) {
    return { label: plainLabel, extensions: [] };
  }

  return { label: "Plain Text", extensions: [] };
}

/* ---------- Custom theme overrides for glassmorphism ---------- */

const axiosTheme = EditorView.theme({
  "&": {
    backgroundColor: "transparent",
    fontSize: "13px",
    fontFamily: "'JetBrains Mono', 'Fira Code', monospace",
  },
  ".cm-gutters": {
    backgroundColor: "transparent",
    borderRight: "1px solid rgba(255, 255, 255, 0.05)",
    color: "rgba(107, 107, 128, 0.6)",
  },
  ".cm-activeLineGutter": {
    backgroundColor: "rgba(99, 102, 241, 0.08)",
    color: "rgba(99, 102, 241, 0.8)",
  },
  ".cm-activeLine": {
    backgroundColor: "rgba(255, 255, 255, 0.03)",
  },
  "&.cm-focused .cm-cursor": {
    borderLeftColor: "#6366f1",
  },
  "&.cm-focused .cm-selectionBackground, .cm-selectionBackground": {
    backgroundColor: "rgba(99, 102, 241, 0.2) !important",
  },
  ".cm-content": {
    caretColor: "#6366f1",
    lineHeight: "1.6",
  },
  ".cm-scroller": {
    overflow: "auto",
  },
  "&.cm-focused": {
    outline: "none",
  },
});

/* ---------- FileEditor Component ---------- */

interface FileEditorProps {
  filePath: string;
  fileName: string;
  onClose: () => void;
}

export function FileEditor({ filePath, fileName, onClose }: FileEditorProps) {
  const editorRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [modified, setModified] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveStatus, setSaveStatus] = useState<"idle" | "saved" | "error">("idle");
  const originalContentRef = useRef<string>("");
  const modifiedRef = useRef(false);

  const langInfo = getLangInfo(fileName);

  // Save handler
  const handleSave = useCallback(async () => {
    const view = viewRef.current;
    if (!view) return;

    const content = view.state.doc.toString();
    setSaving(true);
    setSaveStatus("idle");

    try {
      const res = await fetch("/api/fs/write", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path: filePath, content }),
      });

      if (!res.ok) {
        const data = await res.json().catch(() => ({ error: `HTTP ${res.status}` }));
        throw new Error(data.error || `HTTP ${res.status}`);
      }

      originalContentRef.current = content;
      modifiedRef.current = false;
      setModified(false);
      setSaveStatus("saved");
      toastSuccess("Saved", fileName);
      setTimeout(() => setSaveStatus("idle"), 2000);
    } catch (err: any) {
      setSaveStatus("error");
      setError(`Save failed: ${err.message}`);
      toastError("Save failed", err.message);
      setTimeout(() => {
        setSaveStatus("idle");
        setError(null);
      }, 3000);
    } finally {
      setSaving(false);
    }
  }, [filePath]);

  // Close with unsaved changes check
  const handleClose = useCallback(() => {
    if (modifiedRef.current) {
      const confirmed = window.confirm(
        "You have unsaved changes. Are you sure you want to close?"
      );
      if (!confirmed) return;
    }
    onClose();
  }, [onClose]);

  // Load file content and initialize editor
  useEffect(() => {
    let destroyed = false;

    async function loadFile() {
      setLoading(true);
      setError(null);

      try {
        const res = await fetch(
          `/api/fs/read?path=${encodeURIComponent(filePath)}`
        );
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data: { content: string } = await res.json();

        if (destroyed) return;

        originalContentRef.current = data.content;

        // Create editor
        if (editorRef.current) {
          // Clean up existing editor
          if (viewRef.current) {
            viewRef.current.destroy();
            viewRef.current = null;
          }

          const state = EditorState.create({
            doc: data.content,
            extensions: [
              lineNumbers(),
              highlightActiveLine(),
              highlightActiveLineGutter(),
              bracketMatching(),
              indentOnInput(),
              oneDark,
              axiosTheme,
              ...langInfo.extensions,
              keymap.of([
                {
                  key: "Mod-s",
                  run: () => {
                    handleSave();
                    return true;
                  },
                },
              ]),
              EditorView.updateListener.of((update) => {
                if (update.docChanged) {
                  const currentContent = update.state.doc.toString();
                  const isModified = currentContent !== originalContentRef.current;
                  if (isModified !== modifiedRef.current) {
                    modifiedRef.current = isModified;
                    setModified(isModified);
                  }
                }
              }),
            ],
          });

          const view = new EditorView({
            state,
            parent: editorRef.current,
          });

          viewRef.current = view;
        }
      } catch (err: any) {
        if (!destroyed) {
          setError(err.message);
        }
      } finally {
        if (!destroyed) {
          setLoading(false);
        }
      }
    }

    loadFile();

    return () => {
      destroyed = true;
      if (viewRef.current) {
        viewRef.current.destroy();
        viewRef.current = null;
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filePath]);

  // Global keyboard shortcut for Cmd+S
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "s") {
        e.preventDefault();
        handleSave();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [handleSave]);

  return (
    <div className="flex flex-col h-full">
      {/* Header bar */}
      <div className="flex items-center gap-2 px-3 h-10 border-b border-border glass-subtle shrink-0">
        {/* Back button */}
        <button
          onClick={handleClose}
          className="p-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-white/[0.06] transition-colors"
          title="Close editor"
        >
          <svg
            width="14"
            height="14"
            viewBox="0 0 16 16"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
          >
            <path d="M10 3L5 8l5 5" />
          </svg>
        </button>

        {/* Separator */}
        <div className="w-px h-4 bg-border" />

        {/* File icon + name */}
        <div className="flex items-center gap-2 min-w-0">
          <FileIcon name={fileName} isDir={false} size="sm" />
          <span className="text-xs font-medium text-foreground truncate">
            {fileName}
          </span>

          {/* Modified dot indicator */}
          {modified && (
            <span
              className="w-2 h-2 rounded-full bg-amber-500 shrink-0"
              title="Unsaved changes"
            />
          )}
        </div>

        {/* File path */}
        <span className="text-[10px] text-muted-foreground/50 font-mono truncate hidden sm:inline">
          {filePath}
        </span>

        {/* Spacer */}
        <div className="flex-1" />

        {/* Language badge */}
        <span className="px-2 py-0.5 rounded-full text-[10px] font-medium bg-white/[0.06] text-muted-foreground border border-border shrink-0">
          {langInfo.label}
        </span>

        {/* Save status / error toast */}
        {saveStatus === "saved" && (
          <span className="text-[10px] text-green-400 font-medium shrink-0 animate-pulse">
            Saved
          </span>
        )}
        {saveStatus === "error" && (
          <span className="text-[10px] text-red-400 font-medium shrink-0">
            Error
          </span>
        )}

        {/* Save button */}
        <button
          onClick={handleSave}
          disabled={!modified || saving}
          className={`flex items-center gap-1.5 px-3 py-1 rounded-md text-xs font-medium transition-all duration-150 shrink-0 ${
            modified
              ? "bg-primary text-primary-foreground hover:bg-primary/90 glow-sm"
              : "bg-white/[0.04] text-muted-foreground/40 cursor-not-allowed"
          }`}
          title="Save (Cmd+S)"
        >
          {saving ? (
            <div className="w-3 h-3 border border-primary-foreground/30 border-t-primary-foreground rounded-full animate-spin" />
          ) : (
            <svg
              width="12"
              height="12"
              viewBox="0 0 16 16"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
            >
              <path d="M12.5 14.5h-9a1 1 0 01-1-1v-11a1 1 0 011-1h7l3 3v9a1 1 0 01-1 1z" />
              <path d="M10.5 14.5v-4h-5v4" />
              <path d="M5.5 1.5v3h4" />
            </svg>
          )}
          Save
        </button>

        {/* Keyboard shortcut hint */}
        <kbd className="hidden md:inline-block px-1.5 py-0.5 rounded text-[9px] font-mono text-muted-foreground/30 bg-white/[0.03] border border-border">
          {navigator.platform?.includes("Mac") ? "\u2318S" : "Ctrl+S"}
        </kbd>
      </div>

      {/* Error banner */}
      {error && (
        <div className="px-4 py-2 bg-destructive/10 border-b border-destructive/20 text-xs text-destructive">
          {error}
        </div>
      )}

      {/* Editor area */}
      <div className="flex-1 min-h-0 overflow-hidden">
        {loading && (
          <div className="flex items-center justify-center h-full">
            <div className="flex items-center gap-3">
              <div className="w-4 h-4 border-2 border-border border-t-primary rounded-full animate-spin" />
              <span className="text-xs text-muted-foreground">Loading file...</span>
            </div>
          </div>
        )}
        <div
          ref={editorRef}
          className="h-full w-full overflow-auto"
          style={{ display: loading ? "none" : "block" }}
        />
      </div>
    </div>
  );
}
