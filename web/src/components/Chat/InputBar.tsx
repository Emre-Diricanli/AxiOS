import { useState, type FormEvent, type KeyboardEvent } from "react";

interface InputBarProps {
  onSend: (message: string) => void;
  disabled?: boolean;
}

export function InputBar({ onSend, disabled }: InputBarProps) {
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

  return (
    <form onSubmit={handleSubmit} className="flex gap-2 p-3 border-t border-border shrink-0">
      <textarea
        value={input}
        onChange={(e) => setInput(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder="Message Claude..."
        disabled={disabled}
        rows={1}
        className="flex-1 resize-none rounded-xl glass-subtle px-3.5 py-2.5 text-[13px] text-foreground placeholder:text-muted-foreground outline-none focus:ring-1 focus:ring-primary/30 focus:border-primary/30 disabled:opacity-30 transition-all"
      />
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
    </form>
  );
}
