import { useEffect, useRef } from "react";
import type { FileEntry } from "@/types/messages";

interface ContextMenuProps {
  x: number;
  y: number;
  entry: FileEntry | null; // null = right-clicked on empty space
  currentPath: string;
  onClose: () => void;
  onAction: (action: string, entry?: FileEntry) => void;
  hasClipboard: boolean; // whether something has been copied
}

const EDITABLE_EXTENSIONS = new Set([
  "txt","md","js","ts","jsx","tsx","py","go","rs","java","c","cpp",
  "h","cs","rb","php","swift","kt","sh","bash","zsh","json","yaml",
  "yml","toml","xml","html","css","scss","sql","dockerfile","makefile",
  "gitignore","env","cfg","conf","ini","log","svg","vue","svelte","r",
  "lua","csv","mod","sum","lock","mdx",
]);

function isTextFile(name: string): boolean {
  if (!name.includes(".")) return EDITABLE_EXTENSIONS.has(name.toLowerCase());
  const ext = name.split(".").pop()?.toLowerCase() ?? "";
  return EDITABLE_EXTENSIONS.has(ext);
}

/* ---- Icons (small inline SVGs) ---- */

function IconOpen() {
  return (
    <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.3">
      <path d="M2 4l2-2h4l1 1h5a1 1 0 011 1v8a1 1 0 01-1 1H2a1 1 0 01-1-1V4z" />
      <path d="M1 7h14" />
    </svg>
  );
}

function IconEdit() {
  return (
    <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.3">
      <path d="M11.5 1.5l3 3-9 9H2.5v-3l9-9z" />
      <path d="M9.5 3.5l3 3" />
    </svg>
  );
}

function IconRename() {
  return (
    <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.3">
      <path d="M2 12h3l8-8-3-3-8 8v3z" />
      <path d="M8 3l3 3" />
    </svg>
  );
}

function IconCopy() {
  return (
    <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.3">
      <rect x="5" y="5" width="9" height="9" rx="1" />
      <path d="M11 5V3a1 1 0 00-1-1H3a1 1 0 00-1 1v7a1 1 0 001 1h2" />
    </svg>
  );
}

function IconMove() {
  return (
    <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.3">
      <path d="M8 2v12M5 11l3 3 3-3" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M2 8h12M11 5l3 3-3 3" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function IconDownload() {
  return (
    <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.3">
      <path d="M8 2v8M5 7.5L8 10.5 11 7.5" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M2.5 11v2a1.5 1.5 0 001.5 1.5h8a1.5 1.5 0 001.5-1.5v-2" strokeLinecap="round" />
    </svg>
  );
}

function IconDelete() {
  return (
    <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.3">
      <path d="M3 4h10M5 4V3a1 1 0 011-1h4a1 1 0 011 1v1" strokeLinecap="round" />
      <path d="M4 4l1 10h6l1-10" />
      <path d="M6.5 7v4M9.5 7v4" strokeLinecap="round" />
    </svg>
  );
}

function IconNewFolder() {
  return (
    <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.3">
      <path d="M2 4l2-2h4l1 1h5a1 1 0 011 1v8a1 1 0 01-1 1H2a1 1 0 01-1-1V4z" />
      <path d="M8 6.5v4M6 8.5h4" strokeLinecap="round" />
    </svg>
  );
}

function IconNewFile() {
  return (
    <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.3">
      <path d="M4 1.5h5.5L13 5v9a1.5 1.5 0 01-1.5 1.5h-7A1.5 1.5 0 013 14V3a1.5 1.5 0 011-1.5z" />
      <path d="M9 1.5V5.5h4" strokeLinecap="round" />
      <path d="M8 8v4M6 10h4" strokeLinecap="round" />
    </svg>
  );
}

function IconPaste() {
  return (
    <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.3">
      <rect x="3" y="4" width="10" height="11" rx="1" />
      <path d="M6 4V2.5A.5.5 0 016.5 2h3a.5.5 0 01.5.5V4" />
      <path d="M6 8h4M6 10.5h4" strokeLinecap="round" />
    </svg>
  );
}

function IconRefresh() {
  return (
    <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.3">
      <path d="M13 8a5 5 0 1 1-1-3M13 2v3h-3" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

/* ---- Menu Item ---- */

interface MenuItemProps {
  icon: React.ReactNode;
  label: string;
  destructive?: boolean;
  disabled?: boolean;
  onClick: () => void;
}

function MenuItem({ icon, label, destructive, disabled, onClick }: MenuItemProps) {
  return (
    <button
      onClick={(e) => {
        e.stopPropagation();
        if (!disabled) onClick();
      }}
      disabled={disabled}
      className={`w-full flex items-center gap-2.5 px-3 py-1.5 text-xs rounded-lg transition-colors duration-100 ${
        disabled
          ? "text-muted-foreground/30 cursor-default"
          : destructive
            ? "text-red-400 hover:bg-red-500/10 hover:text-red-300"
            : "text-foreground/80 hover:bg-accent hover:text-foreground"
      }`}
    >
      <span className="shrink-0 opacity-70">{icon}</span>
      <span>{label}</span>
    </button>
  );
}

function Separator() {
  return <div className="my-1 border-t border-border" />;
}

/* ---- Context Menu ---- */

export function ContextMenu({
  x,
  y,
  entry,
  onClose,
  onAction,
  hasClipboard,
}: ContextMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null);

  // Adjust position to stay within viewport
  useEffect(() => {
    const el = menuRef.current;
    if (!el) return;

    const rect = el.getBoundingClientRect();
    const pad = 8;

    let adjustedX = x;
    let adjustedY = y;

    if (rect.right > window.innerWidth - pad) {
      adjustedX = window.innerWidth - rect.width - pad;
    }
    if (rect.bottom > window.innerHeight - pad) {
      adjustedY = window.innerHeight - rect.height - pad;
    }
    if (adjustedX < pad) adjustedX = pad;
    if (adjustedY < pad) adjustedY = pad;

    el.style.left = `${adjustedX}px`;
    el.style.top = `${adjustedY}px`;
  }, [x, y]);

  // Close on click outside, Escape, or scroll
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        onClose();
      }
    }
    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    function handleScroll() {
      onClose();
    }

    document.addEventListener("mousedown", handleClick, true);
    document.addEventListener("keydown", handleKey);
    document.addEventListener("scroll", handleScroll, true);

    return () => {
      document.removeEventListener("mousedown", handleClick, true);
      document.removeEventListener("keydown", handleKey);
      document.removeEventListener("scroll", handleScroll, true);
    };
  }, [onClose]);

  const act = (action: string) => {
    onAction(action, entry ?? undefined);
    onClose();
  };

  return (
    <div
      ref={menuRef}
      className="fixed z-[9999] min-w-[180px] p-1.5 rounded-xl shadow-2xl backdrop-blur-xl bg-popover/90 border border-glass-border"
      style={{ left: x, top: y }}
    >
      {entry ? (
        // Right-clicked on a file or folder
        entry.type === "file" ? (
          <>
            <MenuItem icon={<IconOpen />} label="Open" onClick={() => act("open")} />
            {isTextFile(entry.name) && (
              <MenuItem icon={<IconEdit />} label="Open in Editor" onClick={() => act("open-editor")} />
            )}
            <MenuItem icon={<IconRename />} label="Rename..." onClick={() => act("rename")} />
            <MenuItem icon={<IconCopy />} label="Copy" onClick={() => act("copy")} />
            <MenuItem icon={<IconMove />} label="Move to..." onClick={() => act("move")} />
            <MenuItem icon={<IconDownload />} label="Download" onClick={() => act("download")} />
            <Separator />
            <MenuItem icon={<IconDelete />} label="Delete" destructive onClick={() => act("delete")} />
          </>
        ) : (
          <>
            <MenuItem icon={<IconOpen />} label="Open" onClick={() => act("open")} />
            <MenuItem icon={<IconRename />} label="Rename..." onClick={() => act("rename")} />
            <MenuItem icon={<IconCopy />} label="Copy" onClick={() => act("copy")} />
            <MenuItem icon={<IconMove />} label="Move to..." onClick={() => act("move")} />
            <Separator />
            <MenuItem icon={<IconDelete />} label="Delete" destructive onClick={() => act("delete")} />
          </>
        )
      ) : (
        // Right-clicked on empty space
        <>
          <MenuItem icon={<IconNewFolder />} label="New Folder" onClick={() => act("new-folder")} />
          <MenuItem icon={<IconNewFile />} label="New File" onClick={() => act("new-file")} />
          <MenuItem
            icon={<IconPaste />}
            label="Paste"
            disabled={!hasClipboard}
            onClick={() => act("paste")}
          />
          <Separator />
          <MenuItem icon={<IconRefresh />} label="Refresh" onClick={() => act("refresh")} />
        </>
      )}
    </div>
  );
}
