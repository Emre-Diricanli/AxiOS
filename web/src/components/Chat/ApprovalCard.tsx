import { useState } from "react";
import type { ApprovalStatus } from "@/types/messages";

interface ApprovalCardProps {
  toolName: string;
  params: string;
  status: ApprovalStatus;
  onRespond: (approve: boolean) => void;
}

// Params longer than this start collapsed behind a toggle.
const COLLAPSE_THRESHOLD = 280;

export function ApprovalCard({ toolName, params, status, onRespond }: ApprovalCardProps) {
  const pretty = formatParams(params);
  const isLarge = pretty.length > COLLAPSE_THRESHOLD;
  const [expanded, setExpanded] = useState(!isLarge);
  const displayName = toolName.replace("__", " > ");
  const pending = status === "pending";

  return (
    <div className="flex justify-start">
      <div className="max-w-[95%] w-full rounded-lg border border-amber-500/30 bg-amber-500/5 text-xs overflow-hidden">
        {/* Header */}
        <div className="flex items-center gap-2 px-3 py-2">
          <div className="w-3.5 h-3.5 rounded-full bg-amber-500/20 flex items-center justify-center shrink-0">
            <span className="text-amber-400 text-[8px]">&#9888;</span>
          </div>
          <span className="text-[11px] font-medium text-amber-300">
            Approval required
          </span>
          <span className="text-[11px] font-mono text-muted-foreground flex-1 truncate">
            {displayName}
          </span>
          <StatusBadge status={status} />
        </div>

        {/* Params */}
        {isLarge && (
          <button
            onClick={() => setExpanded(!expanded)}
            className="w-full flex items-center gap-1.5 px-3 py-1 text-[10px] text-muted-foreground hover:text-foreground transition-colors"
          >
            <svg
              width="10"
              height="10"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              className={`transition-transform ${expanded ? "rotate-180" : ""}`}
            >
              <path d="M6 9l6 6 6-6" />
            </svg>
            {expanded ? "Hide parameters" : "Show parameters"}
          </button>
        )}
        {expanded && (
          <pre className="px-3 py-2 border-t border-amber-500/15 font-mono text-foreground/60 overflow-x-auto whitespace-pre-wrap break-all text-[11px] max-h-48 overflow-y-auto scrollbar-none">
            {pretty}
          </pre>
        )}

        {/* Actions */}
        <div className="flex items-center gap-2 px-3 py-2 border-t border-amber-500/15">
          <button
            onClick={() => onRespond(true)}
            disabled={!pending}
            className="px-3 py-1.5 rounded-md text-[11px] font-medium bg-emerald-500/15 text-emerald-400 border border-emerald-500/30 hover:bg-emerald-500/25 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
          >
            Approve
          </button>
          <button
            onClick={() => onRespond(false)}
            disabled={!pending}
            className="px-3 py-1.5 rounded-md text-[11px] font-medium bg-red-500/10 text-red-400 border border-red-500/30 hover:bg-red-500/20 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
          >
            Deny
          </button>
          {status === "expired" && (
            <span className="text-[10px] text-muted-foreground ml-auto">
              Request expired without a response
            </span>
          )}
        </div>
      </div>
    </div>
  );
}

function StatusBadge({ status }: { status: ApprovalStatus }) {
  switch (status) {
    case "approved":
      return (
        <span className="px-2 py-0.5 rounded-full bg-emerald-500/15 text-emerald-400 text-[10px] font-medium shrink-0">
          Approved
        </span>
      );
    case "denied":
      return (
        <span className="px-2 py-0.5 rounded-full bg-red-500/15 text-red-400 text-[10px] font-medium shrink-0">
          Denied
        </span>
      );
    case "expired":
      return (
        <span className="px-2 py-0.5 rounded-full bg-white/5 text-muted-foreground text-[10px] font-medium shrink-0">
          Expired
        </span>
      );
    default:
      return (
        <span className="px-2 py-0.5 rounded-full bg-amber-500/15 text-amber-400 text-[10px] font-medium shrink-0 animate-pulse">
          Pending
        </span>
      );
  }
}

function formatParams(raw: string): string {
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}
