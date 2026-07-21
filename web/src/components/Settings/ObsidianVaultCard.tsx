import { type FormEvent, useCallback, useEffect, useState } from "react";
import {
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  Database,
  FileText,
  FolderOpen,
  HardDrive,
  LoaderCircle,
  Pencil,
  RefreshCw,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  formatObsidianBytes,
  getObsidianStatus,
  ObsidianApiError,
  setObsidianVault,
} from "@/lib/obsidian";
import type { ObsidianVaultStatus } from "@/types/obsidian";

export function ObsidianVaultCard() {
  const [status, setStatus] = useState<ObsidianVaultStatus | null>(null);
  const [path, setPath] = useState("");
  const [editing, setEditing] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadStatus = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const nextStatus = await getObsidianStatus();
      setStatus(nextStatus);
      setPath(nextStatus.vault_path ?? "");
      setEditing(!nextStatus.configured || Boolean(nextStatus.error));
      if (nextStatus.error) setError(nextStatus.error);
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : "Could not load vault status");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadStatus();
  }, [loadStatus]);

  const connectVault = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!path.trim()) {
      setError("Enter an absolute directory path.");
      return;
    }

    setSaving(true);
    setError(null);
    try {
      const nextStatus = await setObsidianVault(path.trim());
      setStatus(nextStatus);
      setPath(nextStatus.vault_path ?? path.trim());
      setEditing(false);
    } catch (saveError) {
      if (saveError instanceof ObsidianApiError && saveError.status === 400) {
        setError(saveError.message);
      } else {
        setError(saveError instanceof Error ? saveError.message : "Could not connect the vault");
      }
    } finally {
      setSaving(false);
    }
  };

  return (
    <section id="settings-obsidian" className="scroll-mt-4 border-t border-border py-5">
      <div className="mb-4 flex items-center justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-2 text-sm font-semibold text-foreground">
            <Database className="size-4 text-primary" />
            Obsidian Vault
          </h2>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            Connect a local Markdown vault for notes and AI tools.
          </p>
        </div>
        {!loading && status?.configured && !editing && (
          <Button variant="outline" size="sm" onClick={() => setEditing(true)}>
            <Pencil className="size-3.5" />
            Change vault
          </Button>
        )}
      </div>

      {loading ? (
        <div className="surface-raised flex h-28 items-center justify-center rounded-lg text-sm text-muted-foreground">
          <LoaderCircle className="mr-2 size-4 animate-spin text-primary" />
          Reading vault status
        </div>
      ) : editing || !status?.configured ? (
        <form onSubmit={connectVault} className="surface-raised rounded-lg p-4">
          <label htmlFor="obsidian-vault-path" className="mb-2 block text-xs font-medium text-foreground">
            Absolute vault path
          </label>
          <div className="flex gap-2 max-[520px]:flex-col">
            <Input
              id="obsidian-vault-path"
              value={path}
              onChange={(event) => {
                setPath(event.target.value);
                if (error) setError(null);
              }}
              placeholder="/absolute/path/to/Obsidian"
              className="font-mono text-xs"
              aria-invalid={Boolean(error)}
              aria-describedby={error ? "obsidian-path-error" : "obsidian-path-help"}
              disabled={saving}
            />
            <Button type="submit" disabled={saving} className="shrink-0">
              {saving ? <LoaderCircle className="size-4 animate-spin" /> : <FolderOpen className="size-4" />}
              {status?.configured ? "Use this vault" : "Connect vault"}
            </Button>
            {status?.configured && !status.error && (
              <Button type="button" variant="ghost" onClick={() => {
                setEditing(false);
                setPath(status.vault_path ?? "");
                setError(status.error ?? null);
              }}>
                Cancel
              </Button>
            )}
          </div>
          {error ? (
            <p id="obsidian-path-error" role="alert" className="mt-2 flex items-start gap-2 text-xs leading-5 text-destructive">
              <AlertCircle className="mt-0.5 size-3.5 shrink-0" />
              {error}
            </p>
          ) : (
            <p id="obsidian-path-help" className="mt-2 text-xs leading-5 text-muted-foreground">
              AxiOS reads and writes Markdown files directly. Obsidian does not need to be running.
            </p>
          )}
        </form>
      ) : (
        <div className="surface-raised overflow-hidden rounded-lg">
          <div className="flex items-start gap-3 border-b border-border p-4">
            <div className="grid size-9 shrink-0 place-items-center rounded-lg border border-primary/20 bg-primary/10 text-primary">
              <Database className="size-4" />
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <p className="truncate text-sm font-medium text-foreground">{status.name}</p>
                <span className="inline-flex items-center gap-1 text-[11px] text-emerald-400">
                  <CheckCircle2 className="size-3" /> Connected
                </span>
              </div>
              <p className="mt-1 truncate font-mono text-xs text-muted-foreground" title={status.vault_path}>
                {status.vault_path}
              </p>
            </div>
            <Button variant="ghost" size="icon-sm" onClick={() => void loadStatus()} aria-label="Refresh vault status">
              <RefreshCw className="size-3.5" />
            </Button>
          </div>
          <div className="grid grid-cols-3 divide-x divide-border">
            <VaultMetric icon={<FileText />} label="Notes" value={String(status.notes ?? 0)} />
            <VaultMetric icon={<FolderOpen />} label="Folders" value={String(status.folders ?? 0)} />
            <VaultMetric icon={<HardDrive />} label="Size" value={formatObsidianBytes(status.size_bytes)} />
          </div>
        </div>
      )}

      {!loading && status?.configured && status.looks_like_vault === false && (
        <div className="mt-3 flex items-start gap-2 rounded-lg border border-amber-500/25 bg-amber-500/8 px-3 py-2.5 text-xs leading-5 text-amber-200">
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-amber-400" />
          This folder has no <code className="font-mono">.obsidian/</code> marker. It still works, but double-check the path.
        </div>
      )}
    </section>
  );
}

function VaultMetric({ icon, label, value }: { icon: React.ReactNode; label: string; value: string }) {
  return (
    <div className="px-4 py-3">
      <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground [&_svg]:size-3">
        {icon}
        {label}
      </div>
      <p className="mt-1 text-sm font-medium text-foreground">{value}</p>
    </div>
  );
}
