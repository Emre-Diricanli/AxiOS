import { useMemo, useState } from "react";
import type { ApprovalStatus } from "@/types/messages";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

interface ApprovalCardProps {
  toolName: string;
  params: string;
  status: ApprovalStatus;
  onRespond: (approve: boolean) => void;
}

interface ImpactSummary {
  risk: "Low" | "Medium" | "High";
  paths: string[];
  command?: string;
  reversible: string;
}

const COLLAPSE_THRESHOLD = 240;

function parsedParams(raw: string): unknown {
  try {
    return JSON.parse(raw);
  } catch {
    return raw;
  }
}

function collectValues(value: unknown, keys: Set<string>, output: string[]): void {
  if (!value || typeof value !== "object") return;
  for (const [key, child] of Object.entries(value as Record<string, unknown>)) {
    if (keys.has(key.toLowerCase()) && typeof child === "string" && child.trim()) output.push(child);
    else if (typeof child === "object") collectValues(child, keys, output);
  }
}

function summarizeImpact(toolName: string, params: unknown): ImpactSummary {
  const paths: string[] = [];
  const commands: string[] = [];
  collectValues(params, new Set(["path", "file", "target", "directory", "cwd"]), paths);
  collectValues(params, new Set(["command", "cmd", "script"]), commands);
  const normalized = `${toolName} ${commands.join(" ")}`.toLowerCase();
  const destructive = /\b(rm|rmdir|dd|mkfs|shutdown|reboot|chmod|chown|truncate)\b|delete|remove|overwrite|docker\s+(rm|prune)/.test(normalized);
  const mutating = destructive || /write|edit|move|rename|mkdir|bash|shell|install|start|stop|restart/.test(normalized);
  return {
    risk: destructive ? "High" : mutating ? "Medium" : "Low",
    paths: [...new Set(paths)].slice(0, 4),
    command: commands[0],
    reversible: destructive ? "No — may be irreversible" : mutating ? "Possibly — review changes or backups" : "Yes — read-only operation",
  };
}

function statusVariant(status: ApprovalStatus): "secondary" | "destructive" | "outline" {
  if (status === "denied") return "destructive";
  if (status === "approved") return "secondary";
  return "outline";
}

export function ApprovalCard({ toolName, params, status, onRespond }: ApprovalCardProps) {
  const parsed = useMemo(() => parsedParams(params), [params]);
  const pretty = typeof parsed === "string" ? parsed : JSON.stringify(parsed, null, 2);
  const impact = useMemo(() => summarizeImpact(toolName, parsed), [parsed, toolName]);
  const [expanded, setExpanded] = useState(pretty.length <= COLLAPSE_THRESHOLD);
  const pending = status === "pending";
  const riskClass = impact.risk === "High"
    ? "text-red-300 border-red-400/30 bg-red-400/10"
    : impact.risk === "Medium"
      ? "text-amber-300 border-amber-400/30 bg-amber-400/10"
      : "text-emerald-300 border-emerald-400/30 bg-emerald-400/10";

  return (
    <div className="flex justify-start">
      <section className={`max-w-[98%] w-full rounded-xl border-2 overflow-hidden shadow-lg ${pending ? "border-amber-400/45 bg-amber-400/[0.06]" : "border-border bg-surface"}`}>
        <header className="flex items-center gap-3 px-4 py-3 border-b border-border">
          <div className="w-9 h-9 rounded-xl bg-amber-400/15 text-amber-300 flex items-center justify-center shrink-0" aria-hidden="true">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M12 3 2 21h20L12 3Z" /><path d="M12 9v5M12 18h.01" />
            </svg>
          </div>
          <div className="min-w-0 flex-1">
            <h3 className="text-sm font-semibold text-foreground">Approval required</h3>
            <p className="text-xs font-mono text-muted-foreground truncate mt-0.5">{toolName.replace("__", " › ")}</p>
          </div>
          <Badge variant={statusVariant(status)} className={pending ? "border-amber-400/30 text-amber-300 animate-pulse" : ""}>
            {status[0].toUpperCase() + status.slice(1)}
          </Badge>
        </header>

        <div className="grid grid-cols-2 gap-px bg-border border-b border-border">
          <div className="bg-surface px-4 py-3">
            <p className="text-xs text-muted-foreground mb-1">Command risk</p>
            <Badge variant="outline" className={riskClass}>{impact.risk}</Badge>
          </div>
          <div className="bg-surface px-4 py-3">
            <p className="text-xs text-muted-foreground mb-1">Target host</p>
            <p className="text-sm font-mono truncate">{window.location.hostname || "localhost"}</p>
          </div>
          <div className="bg-surface px-4 py-3 col-span-2">
            <p className="text-xs text-muted-foreground mb-1">Reversible</p>
            <p className="text-sm text-foreground/85">{impact.reversible}</p>
          </div>
        </div>

        {(impact.command || impact.paths.length > 0) && (
          <div className="px-4 py-3 border-b border-border space-y-2">
            {impact.command && (
              <div>
                <p className="text-xs text-muted-foreground mb-1">Command</p>
                <code className="block rounded-lg bg-workspace px-3 py-2 text-xs font-mono text-foreground/80 break-all">{impact.command}</code>
              </div>
            )}
            {impact.paths.length > 0 && (
              <div>
                <p className="text-xs text-muted-foreground mb-1">Affected files or paths</p>
                <div className="flex flex-wrap gap-1.5">
                  {impact.paths.map((path) => <Badge key={path} variant="secondary" className="font-mono max-w-full truncate">{path}</Badge>)}
                </div>
              </div>
            )}
          </div>
        )}

        {pretty.length > COLLAPSE_THRESHOLD && (
          <Button variant="ghost" size="sm" onClick={() => setExpanded(!expanded)} className="mx-3 my-1">
            {expanded ? "Hide raw parameters" : "Review raw parameters"}
          </Button>
        )}
        {expanded && (
          <pre className="px-4 py-3 border-t border-border font-mono text-xs text-foreground/65 overflow-x-auto whitespace-pre-wrap break-all max-h-48 overflow-y-auto scrollbar-none">
            {pretty}
          </pre>
        )}

        <footer className="flex items-center gap-2 px-4 py-3 border-t border-border bg-workspace/50">
          <Button onClick={() => onRespond(true)} disabled={!pending} className="bg-emerald-600 hover:bg-emerald-500">
            Approve action
          </Button>
          <Button variant="destructive" onClick={() => onRespond(false)} disabled={!pending}>Deny</Button>
          {status === "expired" && <span className="text-xs text-muted-foreground ml-auto">Expired without a response</span>}
        </footer>
      </section>
    </div>
  );
}
