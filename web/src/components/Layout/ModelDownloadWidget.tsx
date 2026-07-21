import { useState } from "react";
import { Check, ChevronDown, ChevronUp, Download, X } from "lucide-react";
import { Progress } from "@/components/ui/progress";
import { useModelDownloads, type ModelDownload } from "@/contexts/ModelDownloadContext";

function DownloadStatusIcon({ download }: { download: ModelDownload }) {
  if (download.state === "complete") {
    return <Check className="w-3.5 h-3.5 text-emerald-400" aria-hidden="true" />;
  }
  if (download.state === "error") {
    return <X className="w-3.5 h-3.5 text-destructive" aria-hidden="true" />;
  }
  return <Download className="w-3.5 h-3.5 text-primary animate-pulse" aria-hidden="true" />;
}

export function ModelDownloadWidget() {
  const { downloads, dismissDownload, clearFinished } = useModelDownloads();
  const [collapsed, setCollapsed] = useState(false);
  const activeCount = downloads.filter((download) => download.state === "starting" || download.state === "downloading").length;
  const finishedCount = downloads.length - activeCount;

  if (downloads.length === 0) return null;

  return (
    <aside className="fixed bottom-5 left-[244px] max-[900px]:left-20 max-[640px]:left-3 max-[640px]:right-3 max-[640px]:bottom-20 z-50 w-80 max-[640px]:w-auto glass rounded-lg shadow-2xl overflow-hidden" aria-label="Model downloads">
      <button
        type="button"
        onClick={() => setCollapsed((value) => !value)}
        className="w-full px-3 py-2.5 flex items-center gap-2 text-left hover:bg-white/[0.03] transition-colors"
        aria-expanded={!collapsed}
      >
        <span className="relative flex items-center justify-center w-7 h-7 rounded-md bg-primary/10 text-primary shrink-0">
          <Download className="w-4 h-4" aria-hidden="true" />
          {activeCount > 0 && <span className="absolute -right-0.5 -top-0.5 w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />}
        </span>
        <span className="flex-1 min-w-0">
          <span className="block text-xs font-medium text-foreground">
            {activeCount > 0 ? `${activeCount} model download${activeCount === 1 ? "" : "s"}` : "Model downloads"}
          </span>
          <span className="block text-[10px] text-muted-foreground">
            {activeCount > 0 ? "Downloads continue while you navigate" : `${finishedCount} finished`}
          </span>
        </span>
        {collapsed
          ? <ChevronUp className="w-4 h-4 text-muted-foreground" aria-hidden="true" />
          : <ChevronDown className="w-4 h-4 text-muted-foreground" aria-hidden="true" />}
      </button>

      {!collapsed && (
        <div className="border-t border-border max-h-72 overflow-y-auto scrollbar-none">
          {downloads.map((download) => {
            const active = download.state === "starting" || download.state === "downloading";
            return (
              <div key={download.key} className="px-3 py-2.5 border-b border-border last:border-b-0">
                <div className="flex items-start gap-2">
                  <span className="mt-0.5 shrink-0"><DownloadStatusIcon download={download} /></span>
                  <div className="flex-1 min-w-0">
                    <p className="text-[11px] font-medium text-foreground truncate" title={download.name}>{download.name}</p>
                    <div className="mt-0.5 flex items-center justify-between gap-2 text-[10px]">
                      <span className={`truncate ${download.state === "error" ? "text-destructive" : "text-muted-foreground"}`} title={download.progress.status}>
                        {download.progress.status}
                      </span>
                      {active && <span className="font-mono text-primary shrink-0">{Math.max(0, download.progress.percent).toFixed(0)}%</span>}
                    </div>
                    <p className="mt-0.5 text-[9px] text-muted-foreground/70 truncate">{download.hostName}</p>
                    {active && <Progress value={download.progress.percent} className="mt-2" />}
                  </div>
                  {!active && (
                    <button
                      type="button"
                      onClick={() => dismissDownload(download.key)}
                      className="w-5 h-5 shrink-0 rounded flex items-center justify-center text-muted-foreground hover:text-foreground hover:bg-white/[0.06]"
                      aria-label={`Dismiss ${download.name}`}
                    >
                      <X className="w-3 h-3" aria-hidden="true" />
                    </button>
                  )}
                </div>
              </div>
            );
          })}
          {finishedCount > 1 && (
            <button
              type="button"
              onClick={clearFinished}
              className="w-full px-3 py-2 text-[10px] text-muted-foreground hover:text-foreground hover:bg-white/[0.03] transition-colors"
            >
              Clear finished
            </button>
          )}
        </div>
      )}
    </aside>
  );
}
