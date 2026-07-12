import { useState } from "react";

export interface DiffFile {
  file?: string;
  patch?: string;
  additions: number;
  deletions: number;
  status?: string;
}

// Renders one unified patch with +/- line coloring.
function PatchView({ patch }: { patch: string }) {
  return (
    <pre className="text-[10px] font-mono leading-relaxed overflow-x-auto whitespace-pre px-2 py-1.5 scrollbar-none">
      {patch.split("\n").map((line, i) => {
        let cls = "text-muted-foreground/80";
        if (line.startsWith("+") && !line.startsWith("+++")) cls = "text-emerald-400/90 bg-emerald-500/5";
        else if (line.startsWith("-") && !line.startsWith("---")) cls = "text-red-400/90 bg-red-500/5";
        else if (line.startsWith("@@")) cls = "text-blue-400/70";
        return (
          <span key={i} className={`block ${cls}`}>
            {line || " "}
          </span>
        );
      })}
    </pre>
  );
}

// Collapsible summary of the file changes a code turn produced.
export function DiffBlock({ files }: { files: DiffFile[] }) {
  const [open, setOpen] = useState(false);
  const [openFile, setOpenFile] = useState<number | null>(null);

  const additions = files.reduce((n, f) => n + (f.additions ?? 0), 0);
  const deletions = files.reduce((n, f) => n + (f.deletions ?? 0), 0);

  return (
    <div className="flex justify-start">
      <div className="max-w-[90%] w-full rounded-xl glass-subtle border border-border/60 overflow-hidden">
        <button
          onClick={() => setOpen(!open)}
          className="w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-accent/40 transition-colors"
        >
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-primary shrink-0">
            <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z" />
            <polyline points="14 2 14 8 20 8" />
          </svg>
          <span className="text-[11px] font-medium text-foreground/80 flex-1">
            {files.length} file{files.length === 1 ? "" : "s"} changed
          </span>
          <span className="text-[10px] font-mono text-emerald-400">+{additions}</span>
          <span className="text-[10px] font-mono text-red-400">−{deletions}</span>
          <svg
            width="10"
            height="10"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2.5"
            className={`text-muted-foreground shrink-0 transition-transform ${open ? "rotate-180" : ""}`}
          >
            <path d="M6 9l6 6 6-6" />
          </svg>
        </button>

        {open &&
          files.map((f, i) => (
            <div key={`${f.file}-${i}`} className="border-t border-border/50">
              <button
                onClick={() => setOpenFile(openFile === i ? null : i)}
                className="w-full flex items-center gap-2 px-3 py-1.5 text-left hover:bg-accent/30 transition-colors"
              >
                <span
                  className={`text-[9px] font-bold uppercase shrink-0 ${
                    f.status === "added"
                      ? "text-emerald-400"
                      : f.status === "deleted"
                        ? "text-red-400"
                        : "text-blue-400"
                  }`}
                >
                  {(f.status ?? "M").charAt(0)}
                </span>
                <span className="text-[11px] font-mono text-foreground/70 truncate flex-1">{f.file}</span>
                <span className="text-[10px] font-mono text-emerald-400/80">+{f.additions}</span>
                <span className="text-[10px] font-mono text-red-400/80">−{f.deletions}</span>
              </button>
              {openFile === i && f.patch && <PatchView patch={f.patch} />}
            </div>
          ))}
      </div>
    </div>
  );
}
