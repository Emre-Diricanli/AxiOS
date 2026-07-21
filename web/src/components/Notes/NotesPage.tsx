import {
  type FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import ReactMarkdown from "react-markdown";
import {
  AlertCircle,
  ArrowLeft,
  ChevronDown,
  ChevronRight,
  Database,
  FileEdit,
  FileText,
  Folder,
  FolderOpen,
  Hash,
  LoaderCircle,
  RefreshCw,
  Save,
  Search,
  Trash2,
  X,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input, Textarea } from "@/components/ui/input";
import {
  deleteObsidianNote,
  formatObsidianBytes,
  getObsidianNote,
  getObsidianStatus,
  listObsidianNotes,
  openObsidianSettings,
  saveObsidianNote,
  searchObsidianNotes,
} from "@/lib/obsidian";
import type {
  ObsidianEntry,
  ObsidianNote,
  ObsidianSearchHit,
  ObsidianVaultStatus,
} from "@/types/obsidian";

function parentFolder(path: string): string {
  const separator = path.lastIndexOf("/");
  return separator === -1 ? "" : path.slice(0, separator);
}

function displayNoteName(name: string): string {
  return name.replace(/\.md$/i, "");
}

function stripFrontmatter(content: string): string {
  return content.replace(/^---\r?\n[\s\S]*?\r?\n---\r?\n?/, "");
}

function formatDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    year: date.getFullYear() === new Date().getFullYear() ? undefined : "numeric",
  }).format(date);
}

function frontmatterValue(value: unknown): string {
  if (Array.isArray(value)) return value.map(String).join(", ");
  if (value && typeof value === "object") return JSON.stringify(value);
  return String(value ?? "");
}

export function NotesPage() {
  const [vault, setVault] = useState<ObsidianVaultStatus | null>(null);
  const [folderCache, setFolderCache] = useState<Record<string, ObsidianEntry[]>>({});
  const [expandedFolders, setExpandedFolders] = useState<Set<string>>(() => new Set([""]));
  const [selectedFolder, setSelectedFolder] = useState("");
  const [entries, setEntries] = useState<ObsidianEntry[]>([]);
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [note, setNote] = useState<ObsidianNote | null>(null);
  const [editorContent, setEditorContent] = useState("");
  const [editing, setEditing] = useState(false);
  const [deleteConfirm, setDeleteConfirm] = useState(false);
  const [pageLoading, setPageLoading] = useState(true);
  const [folderLoading, setFolderLoading] = useState(false);
  const [noteLoading, setNoteLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState("");
  const [tagQuery, setTagQuery] = useState("");
  const [searchHits, setSearchHits] = useState<ObsidianSearchHit[] | null>(null);
  const [searching, setSearching] = useState(false);

  const loadFolder = useCallback(async (folder: string) => {
    const nextEntries = await listObsidianNotes(folder);
    setFolderCache((current) => ({ ...current, [folder]: nextEntries }));
    return nextEntries;
  }, []);

  const refreshSelectedFolder = useCallback(async () => {
    setFolderLoading(true);
    setError(null);
    try {
      const nextEntries = await loadFolder(selectedFolder);
      setEntries(nextEntries);
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : "Could not load notes");
    } finally {
      setFolderLoading(false);
    }
  }, [loadFolder, selectedFolder]);

  useEffect(() => {
    let active = true;
    const initialize = async () => {
      setPageLoading(true);
      try {
        const status = await getObsidianStatus();
        if (!active) return;
        if (!status.configured || status.error) {
          openObsidianSettings();
          return;
        }
        setVault(status);
        const rootEntries = await listObsidianNotes();
        if (!active) return;
        setFolderCache({ "": rootEntries });
        setEntries(rootEntries);
      } catch (loadError) {
        if (active) setError(loadError instanceof Error ? loadError.message : "Could not open the vault");
      } finally {
        if (active) setPageLoading(false);
      }
    };
    void initialize();
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    if (!vault) return;
    void refreshSelectedFolder();
  }, [refreshSelectedFolder, vault]);

  const openNote = useCallback(async (path: string) => {
    setSelectedPath(path);
    setNoteLoading(true);
    setEditing(false);
    setDeleteConfirm(false);
    setError(null);
    try {
      const nextNote = await getObsidianNote(path);
      setNote(nextNote);
      setEditorContent(nextNote.content);
    } catch (loadError) {
      setNote(null);
      setError(loadError instanceof Error ? loadError.message : "Could not open note");
    } finally {
      setNoteLoading(false);
    }
  }, []);

  const selectFolder = (folder: string) => {
    setSelectedFolder(folder);
    setSearchHits(null);
    setSelectedPath(null);
    setNote(null);
    setEditing(false);
  };

  const toggleFolder = async (folder: string) => {
    if (expandedFolders.has(folder)) {
      setExpandedFolders((current) => {
        const next = new Set(current);
        next.delete(folder);
        return next;
      });
      return;
    }

    setExpandedFolders((current) => new Set(current).add(folder));
    if (!folderCache[folder]) {
      try {
        await loadFolder(folder);
      } catch (loadError) {
        setError(loadError instanceof Error ? loadError.message : "Could not open folder");
      }
    }
  };

  const performSearch = useCallback(async (query: string, tag: string) => {
    if (!query.trim() && !tag.trim()) {
      setSearchHits(null);
      return;
    }
    setSearching(true);
    setError(null);
    try {
      setSearchHits(await searchObsidianNotes(query, tag));
    } catch (searchError) {
      setError(searchError instanceof Error ? searchError.message : "Search failed");
    } finally {
      setSearching(false);
    }
  }, []);

  const submitSearch = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    void performSearch(searchQuery, tagQuery);
  };

  const openSearchHit = (hit: ObsidianSearchHit) => {
    const folder = parentFolder(hit.path);
    setSelectedFolder(folder);
    void openNote(hit.path);
  };

  const searchTag = (tag: string) => {
    setSearchQuery("");
    setTagQuery(tag);
    void performSearch("", tag);
  };

  const saveNote = async () => {
    if (!note) return;
    setSaving(true);
    setError(null);
    try {
      await saveObsidianNote(note.path, editorContent);
      const refreshed = await getObsidianNote(note.path);
      setNote(refreshed);
      setEditorContent(refreshed.content);
      setEditing(false);
      await refreshSelectedFolder();
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : "Could not save note");
    } finally {
      setSaving(false);
    }
  };

  const deleteNote = async () => {
    if (!note) return;
    setDeleting(true);
    setError(null);
    try {
      await deleteObsidianNote(note.path);
      setSelectedPath(null);
      setNote(null);
      setEditorContent("");
      setEditing(false);
      setDeleteConfirm(false);
      await refreshSelectedFolder();
      if (searchHits) await performSearch(searchQuery, tagQuery);
    } catch (deleteError) {
      setError(deleteError instanceof Error ? deleteError.message : "Could not delete note");
    } finally {
      setDeleting(false);
    }
  };

  const noteEntries = useMemo(() => entries.filter((entry) => !entry.is_folder), [entries]);
  const dirty = Boolean(note && editorContent !== note.content);

  if (pageLoading) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        <LoaderCircle className="mr-2 size-4 animate-spin text-primary" />
        Opening Obsidian vault
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col bg-workspace">
      <header className="flex min-h-16 shrink-0 items-center justify-between gap-4 border-b border-border bg-background px-5 py-3 max-[640px]:px-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <Database className="size-4 text-primary" />
            <h2 className="truncate text-sm font-semibold text-foreground">{vault?.name ?? "Obsidian Notes"}</h2>
            <Badge variant="outline" className="text-[10px] text-muted-foreground">
              {vault?.notes ?? 0} notes
            </Badge>
          </div>
          <p className="mt-1 truncate font-mono text-[11px] text-muted-foreground">{vault?.vault_path}</p>
        </div>
        <Button variant="ghost" size="icon" onClick={() => void refreshSelectedFolder()} aria-label="Refresh notes">
          <RefreshCw className="size-4" />
        </Button>
      </header>

      <form onSubmit={submitSearch} className="flex shrink-0 items-center gap-2 border-b border-border bg-background px-5 py-2.5 max-[640px]:px-3">
        <div className="relative min-w-0 flex-1">
          <Search className="absolute left-3 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={searchQuery}
            onChange={(event) => setSearchQuery(event.target.value)}
            placeholder="Search note titles and content"
            className="pl-9"
          />
        </div>
        <div className="relative w-40 max-[640px]:w-28">
          <Hash className="absolute left-3 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={tagQuery}
            onChange={(event) => setTagQuery(event.target.value.replace(/^#/, ""))}
            placeholder="Tag"
            className="pl-9"
          />
        </div>
        <Button type="submit" variant="secondary" disabled={searching || (!searchQuery.trim() && !tagQuery.trim())}>
          {searching ? <LoaderCircle className="size-4 animate-spin" /> : <Search className="size-4" />}
          <span className="max-[520px]:hidden">Search</span>
        </Button>
        {searchHits && (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            onClick={() => {
              setSearchHits(null);
              setSearchQuery("");
              setTagQuery("");
            }}
            aria-label="Clear search"
          >
            <X className="size-4" />
          </Button>
        )}
      </form>

      {error && (
        <div className="flex shrink-0 items-center gap-2 border-b border-destructive/20 bg-destructive/8 px-5 py-2 text-xs text-destructive">
          <AlertCircle className="size-3.5 shrink-0" />
          <span className="flex-1">{error}</span>
          <button type="button" onClick={() => setError(null)} aria-label="Dismiss error">
            <X className="size-3.5" />
          </button>
        </div>
      )}

      <div className="grid min-h-0 flex-1 grid-cols-[220px_300px_minmax(0,1fr)] max-[1100px]:grid-cols-[200px_260px_minmax(0,1fr)] max-[760px]:grid-cols-1">
        <aside className="min-h-0 overflow-y-auto border-r border-border bg-sidebar p-2 scrollbar-none max-[760px]:hidden" aria-label="Vault folders">
          <FolderTree
            vaultName={vault?.name ?? "Vault"}
            selectedFolder={selectedFolder}
            folderCache={folderCache}
            expandedFolders={expandedFolders}
            onSelect={selectFolder}
            onToggle={(folder) => void toggleFolder(folder)}
          />
        </aside>

        <section className={`min-h-0 overflow-y-auto border-r border-border bg-background scrollbar-none ${selectedPath ? "max-[760px]:hidden" : ""}`} aria-label="Notes list">
          <div className="sticky top-0 z-10 flex h-10 items-center justify-between border-b border-border bg-background/95 px-3 backdrop-blur-sm">
            <p className="truncate text-xs font-medium text-foreground">
              {searchHits ? `Search results (${searchHits.length})` : selectedFolder || "Vault root"}
            </p>
            {!searchHits && <span className="text-[10px] text-muted-foreground">{noteEntries.length}</span>}
          </div>
          {searching || folderLoading ? (
            <PaneLoading label={searching ? "Searching vault" : "Loading notes"} />
          ) : searchHits ? (
            <SearchResults hits={searchHits} selectedPath={selectedPath} onOpen={openSearchHit} />
          ) : (
            <NoteList entries={noteEntries} selectedPath={selectedPath} onOpen={(path) => void openNote(path)} />
          )}
        </section>

        <main className={`relative min-h-0 bg-workspace ${!selectedPath ? "max-[760px]:hidden" : ""}`}>
          {noteLoading ? (
            <PaneLoading label="Opening note" />
          ) : note ? (
            <NoteViewer
              note={note}
              content={editorContent}
              editing={editing}
              dirty={dirty}
              saving={saving}
              deleting={deleting}
              deleteConfirm={deleteConfirm}
              onBack={() => {
                setSelectedPath(null);
                setNote(null);
              }}
              onEdit={() => setEditing(true)}
              onCancelEdit={() => {
                setEditorContent(note.content);
                setEditing(false);
              }}
              onContentChange={setEditorContent}
              onSave={() => void saveNote()}
              onDeleteRequest={() => setDeleteConfirm(true)}
              onDeleteCancel={() => setDeleteConfirm(false)}
              onDelete={() => void deleteNote()}
              onTag={searchTag}
            />
          ) : (
            <EmptyViewer />
          )}
        </main>
      </div>
    </div>
  );
}

interface FolderTreeProps {
  vaultName: string;
  selectedFolder: string;
  folderCache: Record<string, ObsidianEntry[]>;
  expandedFolders: Set<string>;
  onSelect: (folder: string) => void;
  onToggle: (folder: string) => void;
}

function FolderTree(props: FolderTreeProps) {
  return (
    <div className="space-y-0.5 text-xs">
      <FolderRow path="" name={props.vaultName} level={0} {...props} />
    </div>
  );
}

function FolderRow({
  path,
  name,
  level,
  selectedFolder,
  folderCache,
  expandedFolders,
  onSelect,
  onToggle,
}: FolderTreeProps & { path: string; name: string; level: number }) {
  const expanded = expandedFolders.has(path);
  const folders = (folderCache[path] ?? []).filter((entry) => entry.is_folder);
  return (
    <div>
      <div
        className={`group flex h-8 items-center rounded-md pr-2 transition-colors ${
          selectedFolder === path ? "bg-sidebar-accent text-foreground" : "text-muted-foreground hover:bg-sidebar-accent/70 hover:text-foreground"
        }`}
        style={{ paddingLeft: `${4 + level * 14}px` }}
      >
        <button type="button" onClick={() => onToggle(path)} className="grid size-6 shrink-0 place-items-center" aria-label={`${expanded ? "Collapse" : "Expand"} ${name}`}>
          {expanded ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
        </button>
        <button type="button" onClick={() => onSelect(path)} className="flex min-w-0 flex-1 items-center gap-2 text-left">
          {expanded ? <FolderOpen className="size-3.5 shrink-0 text-primary/80" /> : <Folder className="size-3.5 shrink-0" />}
          <span className="truncate">{name}</span>
        </button>
      </div>
      {expanded && folders.map((folder) => (
        <FolderRow
          key={folder.path}
          path={folder.path}
          name={folder.name}
          level={level + 1}
          selectedFolder={selectedFolder}
          folderCache={folderCache}
          expandedFolders={expandedFolders}
          onSelect={onSelect}
          onToggle={onToggle}
          vaultName={name}
        />
      ))}
    </div>
  );
}

function NoteList({ entries, selectedPath, onOpen }: { entries: ObsidianEntry[]; selectedPath: string | null; onOpen: (path: string) => void }) {
  if (entries.length === 0) {
    return <ListEmpty icon={<FileText />} title="No notes here" detail="Choose another folder or search the vault." />;
  }
  return (
    <div className="divide-y divide-border/70">
      {entries.map((entry) => (
        <button
          key={entry.path}
          type="button"
          onClick={() => onOpen(entry.path)}
          className={`w-full px-3 py-3 text-left transition-colors ${selectedPath === entry.path ? "bg-primary/8" : "hover:bg-surface-hover"}`}
        >
          <div className="flex items-start gap-2.5">
            <FileText className={`mt-0.5 size-3.5 shrink-0 ${selectedPath === entry.path ? "text-primary" : "text-muted-foreground"}`} />
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium text-foreground">{displayNoteName(entry.name)}</p>
              <div className="mt-1 flex items-center justify-between gap-2 text-[10px] text-muted-foreground">
                <span>{formatDate(entry.modified)}</span>
                <span>{formatObsidianBytes(entry.size)}</span>
              </div>
            </div>
          </div>
        </button>
      ))}
    </div>
  );
}

function SearchResults({ hits, selectedPath, onOpen }: { hits: ObsidianSearchHit[]; selectedPath: string | null; onOpen: (hit: ObsidianSearchHit) => void }) {
  if (hits.length === 0) {
    return <ListEmpty icon={<Search />} title="No matches" detail="Try a broader phrase or a different tag." />;
  }
  return (
    <div className="divide-y divide-border/70">
      {hits.map((hit) => (
        <button
          key={hit.path}
          type="button"
          onClick={() => onOpen(hit)}
          className={`w-full px-3 py-3 text-left transition-colors ${selectedPath === hit.path ? "bg-primary/8" : "hover:bg-surface-hover"}`}
        >
          <p className="truncate text-sm font-medium text-foreground">{displayNoteName(hit.name)}</p>
          <p className="mt-1 truncate font-mono text-[10px] text-primary/75">{hit.path}</p>
          <p className="mt-2 line-clamp-3 text-xs leading-5 text-muted-foreground">{hit.snippet || "No preview available"}</p>
          <p className="mt-2 text-[10px] text-muted-foreground">{formatDate(hit.modified)}</p>
        </button>
      ))}
    </div>
  );
}

interface NoteViewerProps {
  note: ObsidianNote;
  content: string;
  editing: boolean;
  dirty: boolean;
  saving: boolean;
  deleting: boolean;
  deleteConfirm: boolean;
  onBack: () => void;
  onEdit: () => void;
  onCancelEdit: () => void;
  onContentChange: (content: string) => void;
  onSave: () => void;
  onDeleteRequest: () => void;
  onDeleteCancel: () => void;
  onDelete: () => void;
  onTag: (tag: string) => void;
}

function NoteViewer({
  note,
  content,
  editing,
  dirty,
  saving,
  deleting,
  deleteConfirm,
  onBack,
  onEdit,
  onCancelEdit,
  onContentChange,
  onSave,
  onDeleteRequest,
  onDeleteCancel,
  onDelete,
  onTag,
}: NoteViewerProps) {
  const frontmatter = Object.entries(note.frontmatter ?? {});
  return (
    <article className="flex h-full min-h-0 flex-col">
      <header className="flex min-h-14 shrink-0 items-center gap-3 border-b border-border bg-background px-4 py-2.5">
        <Button variant="ghost" size="icon-sm" onClick={onBack} className="hidden max-[760px]:inline-flex" aria-label="Back to notes">
          <ArrowLeft className="size-4" />
        </Button>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h2 className="truncate text-sm font-semibold text-foreground">{displayNoteName(note.path.split("/").pop() ?? note.path)}</h2>
            {dirty && <span className="size-1.5 shrink-0 rounded-full bg-amber-400" title="Unsaved changes" />}
          </div>
          <p className="mt-0.5 truncate font-mono text-[10px] text-muted-foreground">{note.path}</p>
        </div>
        <div className="flex items-center gap-1.5">
          {editing ? (
            <>
              <Button variant="ghost" size="sm" onClick={onCancelEdit} disabled={saving}>Cancel</Button>
              <Button size="sm" onClick={onSave} disabled={saving || !dirty}>
                {saving ? <LoaderCircle className="size-3.5 animate-spin" /> : <Save className="size-3.5" />}
                Save
              </Button>
            </>
          ) : (
            <Button variant="outline" size="sm" onClick={onEdit}>
              <FileEdit className="size-3.5" />
              Edit
            </Button>
          )}
          <Button variant="ghost" size="icon-sm" onClick={onDeleteRequest} className="text-muted-foreground hover:text-destructive" aria-label="Delete note">
            <Trash2 className="size-3.5" />
          </Button>
        </div>
      </header>

      {deleteConfirm && (
        <div className="flex shrink-0 items-center gap-3 border-b border-destructive/25 bg-destructive/8 px-4 py-3">
          <AlertCircle className="size-4 shrink-0 text-destructive" />
          <p className="flex-1 text-xs text-foreground">Delete <strong>{note.path}</strong>? This cannot be undone.</p>
          <Button variant="ghost" size="sm" onClick={onDeleteCancel} disabled={deleting}>Cancel</Button>
          <Button variant="destructive" size="sm" onClick={onDelete} disabled={deleting}>
            {deleting ? <LoaderCircle className="size-3.5 animate-spin" /> : <Trash2 className="size-3.5" />}
            Delete
          </Button>
        </div>
      )}

      {editing ? (
        <div className="min-h-0 flex-1 p-3">
          <Textarea
            value={content}
            onChange={(event) => onContentChange(event.target.value)}
            onKeyDown={(event) => {
              if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "s") {
                event.preventDefault();
                if (dirty && !saving) onSave();
              }
            }}
            className="h-full min-h-0 resize-none rounded-md font-mono text-[13px] leading-6"
            spellCheck={false}
            aria-label={`Edit ${note.path}`}
          />
        </div>
      ) : (
        <div className="min-h-0 flex-1 overflow-y-auto px-7 py-6 scrollbar-none max-[640px]:px-4">
          {(note.tags.length > 0 || frontmatter.length > 0) && (
            <div className="mb-6 space-y-3 border-b border-border pb-5">
              {note.tags.length > 0 && (
                <div className="flex flex-wrap gap-1.5">
                  {note.tags.map((tag) => (
                    <button key={tag} type="button" onClick={() => onTag(tag)}>
                      <Badge variant="secondary" className="hover:text-primary">#{tag}</Badge>
                    </button>
                  ))}
                </div>
              )}
              {frontmatter.length > 0 && (
                <div className="flex flex-wrap gap-1.5">
                  {frontmatter.map(([key, value]) => (
                    <Badge key={key} variant="outline" className="max-w-full font-normal text-muted-foreground">
                      <span className="font-medium text-foreground/80">{key}:</span>
                      <span className="truncate">{frontmatterValue(value)}</span>
                    </Badge>
                  ))}
                </div>
              )}
            </div>
          )}
          <div className="prose-axios mx-auto max-w-3xl text-[14px] leading-7">
            <ReactMarkdown>{stripFrontmatter(note.content)}</ReactMarkdown>
          </div>
          <div className="mx-auto mt-10 flex max-w-3xl items-center justify-between border-t border-border pt-4 text-[10px] text-muted-foreground">
            <span>Updated {formatDate(note.modified)}</span>
            <span>{formatObsidianBytes(note.size)}</span>
          </div>
        </div>
      )}
    </article>
  );
}

function EmptyViewer() {
  return (
    <div className="grid h-full place-items-center p-6 text-center">
      <div>
        <div className="mx-auto mb-4 grid size-11 place-items-center rounded-lg border border-border bg-surface text-muted-foreground">
          <FileText className="size-5" />
        </div>
        <p className="text-sm font-medium text-foreground">Select a note</p>
        <p className="mt-1 text-xs leading-5 text-muted-foreground">Browse a folder or search across your vault.</p>
      </div>
    </div>
  );
}

function PaneLoading({ label }: { label: string }) {
  return (
    <div className="flex h-full min-h-40 items-center justify-center text-xs text-muted-foreground">
      <LoaderCircle className="mr-2 size-3.5 animate-spin text-primary" />
      {label}
    </div>
  );
}

function ListEmpty({ icon, title, detail }: { icon: React.ReactNode; title: string; detail: string }) {
  return (
    <div className="px-6 py-12 text-center">
      <div className="mx-auto mb-3 grid size-9 place-items-center rounded-lg border border-border bg-surface text-muted-foreground [&_svg]:size-4">
        {icon}
      </div>
      <p className="text-sm font-medium text-foreground">{title}</p>
      <p className="mt-1 text-xs leading-5 text-muted-foreground">{detail}</p>
    </div>
  );
}
