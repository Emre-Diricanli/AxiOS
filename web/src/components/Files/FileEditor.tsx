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
import Markdown from "react-markdown";
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

/* ---------- AI Quick Actions ---------- */

interface AIAction {
  label: string;
  icon: string;
  prompt: (code: string) => string;
}

const AI_ACTIONS: AIAction[] = [
  {
    label: "Explain",
    icon: "\uD83D\uDCD6",
    prompt: (code) => `Explain what this code does in detail:\n\n\`\`\`\n${code}\n\`\`\``,
  },
  {
    label: "Fix bugs",
    icon: "\uD83D\uDC1B",
    prompt: (code) => `Find and fix bugs in this code:\n\n\`\`\`\n${code}\n\`\`\``,
  },
  {
    label: "Refactor",
    icon: "\u2728",
    prompt: (code) => `Refactor this code to be cleaner and more efficient:\n\n\`\`\`\n${code}\n\`\`\``,
  },
  {
    label: "Add comments",
    icon: "\uD83D\uDCDD",
    prompt: (code) => `Add clear comments to this code:\n\n\`\`\`\n${code}\n\`\`\``,
  },
  {
    label: "Write tests",
    icon: "\uD83E\uDDEA",
    prompt: (code) => `Write unit tests for this code:\n\n\`\`\`\n${code}\n\`\`\``,
  },
];

/* ---------- AI Panel Component ---------- */

interface AIPanelProps {
  viewRef: React.RefObject<EditorView | null>;
  fileName: string;
  onClose: () => void;
}

function AIPanel({ viewRef, fileName, onClose }: AIPanelProps) {
  const [prompt, setPrompt] = useState("");
  const [response, setResponse] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [aiError, setAiError] = useState<string | null>(null);
  const responseRef = useRef<HTMLDivElement>(null);
  const selectionRangeRef = useRef<{ from: number; to: number } | null>(null);

  const getCodeContext = useCallback((): string => {
    const view = viewRef.current;
    if (!view) return "";
    const sel = view.state.selection.main;
    if (sel.from !== sel.to) {
      selectionRangeRef.current = { from: sel.from, to: sel.to };
      return view.state.sliceDoc(sel.from, sel.to);
    }
    selectionRangeRef.current = null;
    return view.state.doc.toString();
  }, [viewRef]);

  const askAI = useCallback(async (question: string) => {
    const context = getCodeContext();
    if (!context && !question) return;

    setLoading(true);
    setAiError(null);
    setResponse(null);

    try {
      const res = await fetch("/api/ai/ask", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt: question, context }),
      });

      if (!res.ok) {
        const data = await res.json().catch(() => ({ error: `HTTP ${res.status}` }));
        throw new Error(data.error || `HTTP ${res.status}`);
      }

      const data: { response: string } = await res.json();
      setResponse(data.response);
    } catch (err: any) {
      setAiError(err.message);
    } finally {
      setLoading(false);
    }
  }, [getCodeContext]);

  const handleQuickAction = useCallback((action: AIAction) => {
    const code = getCodeContext();
    const question = action.prompt(code);
    setPrompt(question);
    askAI(question);
  }, [getCodeContext, askAI]);

  const handleSubmit = useCallback(() => {
    if (!prompt.trim()) return;
    askAI(prompt);
  }, [prompt, askAI]);

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      handleSubmit();
    }
  }, [handleSubmit]);

  // Apply code from AI response to the editor
  const applyCode = useCallback((code: string) => {
    const view = viewRef.current;
    if (!view) return;

    const range = selectionRangeRef.current;
    if (range) {
      // Replace selected region
      view.dispatch({
        changes: { from: range.from, to: range.to, insert: code },
      });
    } else {
      // Replace entire document
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: code },
      });
    }
    toastSuccess("Applied", "AI suggestion applied to editor");
  }, [viewRef]);

  useEffect(() => {
    if (responseRef.current) {
      responseRef.current.scrollTop = responseRef.current.scrollHeight;
    }
  }, [response]);

  return (
    <div className="flex flex-col h-full w-[350px] border-l border-border glass-subtle shrink-0">
      {/* Header */}
      <div className="flex items-center justify-between px-3 h-10 border-b border-border shrink-0">
        <div className="flex items-center gap-2">
          <div className="w-5 h-5 rounded-md bg-gradient-to-br from-primary to-purple-500 flex items-center justify-center">
            <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="white" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M8 1v14M1 8h14M3 3l10 10M13 3L3 13" />
            </svg>
          </div>
          <span className="text-xs font-semibold text-foreground">AI Assistant</span>
        </div>
        <button
          onClick={onClose}
          className="p-1 rounded-md text-muted-foreground hover:text-foreground hover:bg-white/[0.06] transition-colors"
        >
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
            <path d="M4 4l8 8M12 4l-8 8" />
          </svg>
        </button>
      </div>

      {/* Quick actions */}
      <div className="px-3 py-2 border-b border-border shrink-0">
        <p className="text-[10px] text-muted-foreground uppercase tracking-wider mb-1.5">Quick actions</p>
        <div className="flex flex-wrap gap-1">
          {AI_ACTIONS.map((action) => (
            <button
              key={action.label}
              onClick={() => handleQuickAction(action)}
              disabled={loading}
              className="flex items-center gap-1 px-2 py-1 rounded-full text-[11px] font-medium bg-white/[0.04] text-muted-foreground hover:text-foreground hover:bg-white/[0.08] border border-border transition-colors disabled:opacity-40"
            >
              <span className="text-[10px]">{action.icon}</span>
              {action.label}
            </button>
          ))}
        </div>
        <p className="text-[9px] text-muted-foreground/50 mt-1.5">
          {selectionRangeRef.current ? "Using selected text" : "Using entire file"} - {fileName}
        </p>
      </div>

      {/* Custom prompt */}
      <div className="px-3 py-2 border-b border-border shrink-0">
        <textarea
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Ask a question about this code..."
          rows={3}
          className="w-full resize-none rounded-lg px-3 py-2 text-xs bg-white/[0.04] border border-border text-foreground placeholder:text-muted-foreground/40 focus:outline-none focus:border-primary/40 focus:ring-1 focus:ring-primary/20 transition-colors"
        />
        <div className="flex items-center justify-between mt-1.5">
          <kbd className="text-[9px] font-mono text-muted-foreground/30">
            {navigator.platform?.includes("Mac") ? "\u2318" : "Ctrl"}+Enter to send
          </kbd>
          <button
            onClick={handleSubmit}
            disabled={!prompt.trim() || loading}
            className={`flex items-center gap-1.5 px-3 py-1 rounded-md text-xs font-medium transition-all duration-150 ${
              prompt.trim() && !loading
                ? "bg-primary text-primary-foreground hover:bg-primary/90 glow-sm"
                : "bg-white/[0.04] text-muted-foreground/40 cursor-not-allowed"
            }`}
          >
            {loading ? (
              <div className="w-3 h-3 border border-primary-foreground/30 border-t-primary-foreground rounded-full animate-spin" />
            ) : (
              <svg width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M2 8h12M10 4l4 4-4 4" />
              </svg>
            )}
            Ask
          </button>
        </div>
      </div>

      {/* Response area */}
      <div ref={responseRef} className="flex-1 overflow-y-auto scrollbar-none min-h-0">
        {loading && !response && (
          <div className="flex items-center justify-center py-12">
            <div className="flex flex-col items-center gap-3">
              <div className="w-6 h-6 border-2 border-border border-t-primary rounded-full animate-spin" />
              <span className="text-xs text-muted-foreground">Thinking...</span>
            </div>
          </div>
        )}

        {aiError && (
          <div className="mx-3 mt-3 px-3 py-2 rounded-lg bg-destructive/10 border border-destructive/20 text-xs text-red-300">
            {aiError}
          </div>
        )}

        {response && (
          <div className="p-3">
            <div className="prose-axios">
              <Markdown
                components={{
                  pre: ({ children, ...props }) => {
                    // Extract code text from the children
                    let codeText = "";
                    if (children && typeof children === "object" && "props" in (children as any)) {
                      const childProps = (children as any).props;
                      if (typeof childProps?.children === "string") {
                        codeText = childProps.children;
                      }
                    }
                    return (
                      <div className="relative group">
                        <pre {...props}>{children}</pre>
                        {codeText && (
                          <button
                            onClick={() => applyCode(codeText.trimEnd())}
                            className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-medium bg-primary text-primary-foreground hover:bg-primary/90 transition-all duration-150"
                          >
                            <svg width="8" height="8" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                              <polyline points="1 4 1 10 7 10" />
                              <polyline points="15 12 15 6 9 6" />
                            </svg>
                            Apply
                          </button>
                        )}
                      </div>
                    );
                  },
                }}
              >
                {response}
              </Markdown>
            </div>
          </div>
        )}

        {!loading && !response && !aiError && (
          <div className="flex flex-col items-center justify-center py-12 text-center px-4">
            <div className="w-10 h-10 rounded-xl glass flex items-center justify-center mb-3 glow-primary">
              <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" className="text-primary">
                <path d="M8 1v14M1 8h14M3 3l10 10M13 3L3 13" />
              </svg>
            </div>
            <p className="text-xs font-medium text-foreground/60 mb-1">Ask AI about your code</p>
            <p className="text-[10px] text-muted-foreground/50">
              Select code in the editor for targeted analysis, or use the entire file
            </p>
          </div>
        )}
      </div>
    </div>
  );
}

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
  const [aiPanelOpen, setAiPanelOpen] = useState(false);
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

        {/* AI button */}
        <button
          onClick={() => setAiPanelOpen(!aiPanelOpen)}
          className={`flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium transition-all duration-150 shrink-0 ${
            aiPanelOpen
              ? "bg-primary/20 text-primary border-glow"
              : "bg-white/[0.04] text-muted-foreground hover:text-foreground hover:bg-white/[0.08]"
          }`}
          title="AI Assistant"
        >
          <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
            <path d="M8 1v14M1 8h14M3 3l10 10M13 3L3 13" />
          </svg>
          AI
        </button>

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

      {/* Editor + AI panel area */}
      <div className="flex flex-1 min-h-0 overflow-hidden">
        {/* Editor area */}
        <div className="flex-1 min-h-0 min-w-0 overflow-hidden">
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

        {/* AI Panel */}
        {aiPanelOpen && (
          <AIPanel
            viewRef={viewRef}
            fileName={fileName}
            onClose={() => setAiPanelOpen(false)}
          />
        )}
      </div>
    </div>
  );
}
