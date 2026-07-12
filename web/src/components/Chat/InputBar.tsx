import { useState, type FormEvent, type KeyboardEvent } from "react";

interface InputBarProps {
  onSend: (message: string) => void;
  disabled?: boolean;
  modelName?: string;
  codeMode?: boolean;
  onToggleCodeMode?: () => void;
  // Code-lane extras: project directory for the session's first message,
  // and a stop control while a code turn is streaming.
  codeActive?: boolean;
  codeDir?: string;
  onCodeDirChange?: (dir: string) => void;
  streaming?: boolean;
  onAbort?: () => void;
}

export function InputBar({
  onSend,
  disabled,
  modelName,
  codeMode,
  onToggleCodeMode,
  codeActive,
  codeDir,
  onCodeDirChange,
  streaming,
  onAbort,
}: InputBarProps) {
  const [input, setInput] = useState("");

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    const trimmed = input.trim();
    if (!trimmed || disabled) return;
    onSend(trimmed);
    setInput("");
  };

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSubmit(e);
    }
  };

  const showStop = streaming && codeActive && onAbort;

  return (
    <form onSubmit={handleSubmit} className="flex flex-col border-t border-border shrink-0">
      {codeActive && onCodeDirChange && (
        <div className="flex items-center gap-2 px-3 pt-2 animate-fade-up">
          <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-muted-foreground shrink-0">
            <path d="M22 19a2 2 0 01-2 2H4a2 2 0 01-2-2V5a2 2 0 012-2h5l2 3h9a2 2 0 012 2z" />
          </svg>
          <input
            value={codeDir ?? ""}
            onChange={(e) => onCodeDirChange(e.target.value)}
            placeholder="~/axios-workspace (project directory for new code sessions)"
            spellCheck={false}
            className="flex-1 bg-transparent text-[10px] font-mono text-foreground/70 placeholder:text-muted-foreground/50 outline-none"
          />
        </div>
      )}
      <div className="flex gap-2 p-3">
        {onToggleCodeMode && (
          <button
            type="button"
            onClick={onToggleCodeMode}
            title={codeMode ? "Code mode on — messages go to the opencode coding agent" : "Switch to code mode (opencode coding agent)"}
            className={`w-10 h-10 rounded-xl flex items-center justify-center transition-all shrink-0 border ${
              codeMode
                ? "bg-primary/20 text-primary border-primary/40 glow-sm"
                : "glass-subtle text-muted-foreground border-transparent hover:text-foreground"
            }`}
          >
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <polyline points="16 18 22 12 16 6" />
              <polyline points="8 6 2 12 8 18" />
            </svg>
          </button>
        )}
        <textarea
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={codeActive ? "Code with opencode…" : `Message ${modelName ?? "AxiOS"}...`}
          disabled={disabled}
          rows={1}
          className="flex-1 resize-none rounded-xl glass-subtle px-3.5 py-2.5 text-[13px] text-foreground placeholder:text-muted-foreground outline-none focus:ring-1 focus:ring-primary/30 focus:border-primary/30 disabled:opacity-30 transition-all"
        />
        {showStop ? (
          <button
            type="button"
            onClick={onAbort}
            title="Stop the running code turn"
            className="w-10 h-10 rounded-xl bg-destructive/15 text-destructive border border-destructive/30 flex items-center justify-center hover:bg-destructive/25 transition-all shrink-0 animate-scale-in"
          >
            <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
              <rect x="6" y="6" width="12" height="12" rx="2" />
            </svg>
          </button>
        ) : (
          <button
            type="submit"
            disabled={disabled || !input.trim()}
            className="w-10 h-10 rounded-xl bg-primary text-primary-foreground flex items-center justify-center hover:bg-primary/90 disabled:opacity-20 transition-all shadow-[0_2px_12px_rgba(99,102,241,0.3)] disabled:shadow-none shrink-0"
          >
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <line x1="22" y1="2" x2="11" y2="13" />
              <polygon points="22 2 15 22 11 13 2 9 22 2" />
            </svg>
          </button>
        )}
      </div>
    </form>
  );
}
