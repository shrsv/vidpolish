import { useEffect, useRef, useState } from 'preact/hooks';
import {
  Play,
  Trash2,
  Pencil,
  ExternalLink,
  Loader2,
  ChevronDown,
  ChevronRight,
  Download,
  Link2,
  Plus,
  Settings,
  ImageIcon,
  Copy,
  Check,
  Lock,
  LockOpen,
  CircleQuestionMark,
  FileImage,
  FileText,
  Scissors,
  Wind,
  Maximize2,
  Gauge,
  Bookmark,
  Save,
  MoreHorizontal,
  ChevronLeft,
} from 'lucide-preact';
import { api } from '../api.js';
import { navigate, paths } from '../router.js';
import { MediaInfoBadge, formatSize } from './MediaInfoBadge.jsx';
import { sanitizeFilename, saveBlob } from '../download.js';
import { renderMarkdown } from '../markdown.js';
import { timeAgo, fullTimestamp } from '../time.js';

const STATUS_CLASS = {
  idle: 'badge-idle',
  running: 'badge-running',
  done: 'badge-done',
  error: 'badge-error',
};

export function Cell({ cell, editCells, project, collapsed, onToggleCollapse, onChanged, onDelete, onAddUpload }) {
  const [live, setLive] = useState(cell.statusMessage || '');
  const [editingName, setEditingName] = useState(false);
  const [name, setName] = useState(cell.name);
  const [youtubeUrl, setYoutubeUrl] = useState(cell.youtubeUrl || '');
  const stopRef = useRef(null);

  useEffect(() => setYoutubeUrl(cell.youtubeUrl || ''), [cell.youtubeUrl]);

  useEffect(() => {
    if (cell.status !== 'running') return;
    stopRef.current?.();
    stopRef.current = api.subscribeCellEvents(cell.id, (msg) => {
      if (msg === '__done__') {
        onChanged();
        return;
      }
      // The upload's own YouTube link is known well before the whole job
      // finishes (YouTube keeps processing for up to ~2 minutes after);
      // show it immediately instead of waiting for that to complete.
      if (msg.startsWith('uploaded: ')) {
        setYoutubeUrl(msg.slice('uploaded: '.length));
      }
      setLive(msg);
    });

    // The actual progress driver, not just a fallback: SSE doesn't
    // reliably deliver inside the Wails desktop build's embedded webview
    // (confirmed - a real run there showed "running" the whole time with
    // no live text ever arriving, then correctly finished, so the
    // subscription above silently never received anything at all). Poll
    // the real status/statusMessage directly instead; this is what
    // actually keeps the progress text moving and picks up completion
    // even if the SSE message for either never shows up. Left the SSE
    // subscription in place too since it's harmless and does work over a
    // plain browser (`vidpolish ui`), just no longer relied on alone.
    const poll = setInterval(() => {
      api
        .getCell(cell.id)
        .then((c) => {
          if (c.statusMessage) setLive(c.statusMessage);
          if (c.youtubeUrl) setYoutubeUrl(c.youtubeUrl);
          if (c.status !== 'running') onChanged();
        })
        .catch(() => {});
    }, 1000);

    return () => {
      stopRef.current?.();
      clearInterval(poll);
    };
  }, [cell.status, cell.id]);

  const run = () => api.runCell(cell.id).then(onChanged);
  const remove = () => {
    if (confirm(`Delete "${cell.name}"?`)) api.deleteCell(cell.id).then(onDelete);
  };
  const download = async () => {
    const suggestedName = `${sanitizeFilename(cell.name)}.mp4`;
    // Prefer a real OS save-file dialog (Chromium's File System Access
    // API) so people can pick where the file goes, instead of it silently
    // landing in the browser's default Downloads folder.
    if (window.showSaveFilePicker) {
      try {
        const handle = await window.showSaveFilePicker({
          suggestedName,
          types: [{ description: 'MP4 video', accept: { 'video/mp4': ['.mp4'] } }],
        });
        const res = await fetch(cell.mediaUrl);
        const blob = await res.blob();
        const writable = await handle.createWritable();
        await writable.write(blob);
        await writable.close();
        return;
      } catch (e) {
        if (e.name === 'AbortError') return; // user cancelled the picker
        // fall through to the plain download link on any other failure
      }
    }
    const a = document.createElement('a');
    a.href = cell.mediaUrl;
    a.download = suggestedName;
    a.click();
  };
  const saveName = async () => {
    setEditingName(false);
    if (name !== cell.name) await api.updateCell(cell.id, { name });
    onChanged();
  };

  const parent = cell.parentCellId
    ? [...project.cells].find((c) => c.id === cell.parentCellId)
    : null;

  return (
    <div class="card p-4 space-y-3">
      <div class="flex items-center justify-between gap-2">
        <div class="flex items-center gap-2 min-w-0">
          <button
            class="text-slate-500 hover:text-slate-300 transition-colors shrink-0"
            onClick={onToggleCollapse}
            title={collapsed ? 'Expand' : 'Collapse'}
          >
            {collapsed ? <ChevronRight size={16} /> : <ChevronDown size={16} />}
          </button>
          <span class="text-xs font-mono text-slate-500 shrink-0">#{cell.seq}</span>
          {editingName ? (
            <input
              class="input py-0.5 text-sm w-48"
              value={name}
              onInput={(e) => setName(e.currentTarget.value)}
              onBlur={saveName}
              onKeyDown={(e) => e.key === 'Enter' && saveName()}
              autoFocus
            />
          ) : (
            <button class="font-medium flex items-center gap-1 hover:text-cyan-400 min-w-0" onClick={() => setEditingName(true)}>
              <span class="truncate">{cell.name}</span> <Pencil size={12} class="opacity-50 shrink-0" />
            </button>
          )}
          {cell.kind !== 'text' && <span class={STATUS_CLASS[cell.status]}>{cell.status}</span>}
          {parent && (
            <button
              class="text-xs text-slate-500 hover:text-cyan-400 flex items-center gap-1 transition-colors shrink-0"
              onClick={() => navigate(paths.cell(project.id, parent.seq))}
              title={`Based on #${parent.seq} ${parent.name}`}
            >
              <Link2 size={12} /> based on #{parent.seq}
            </button>
          )}
          <span
            class="text-xs text-slate-600 shrink-0"
            title={`Created ${fullTimestamp(cell.createdAt)}\nUpdated ${fullTimestamp(cell.updatedAt)}`}
          >
            updated {timeAgo(cell.updatedAt)}
          </span>
        </div>

        {/* Cell toolbar: one disciplined place for every action. */}
        <div class="flex items-center gap-2 shrink-0">
          {cell.kind !== 'text' && (
            <button class="btn-primary" disabled={cell.status === 'running'} onClick={run}>
              {cell.status === 'running' ? <Loader2 size={15} class="animate-spin" /> : <Play size={15} />}
              Run
            </button>
          )}
          {cell.kind === 'edit' && cell.mediaUrl && (
            <button class="btn-secondary" onClick={download} title="Download">
              <Download size={15} />
            </button>
          )}
          {cell.kind === 'edit' && cell.mediaUrl && <GifExportButton cell={cell} />}
          {cell.kind === 'edit' && <ProfileMenuButton cell={cell} onChanged={onChanged} />}
          <button class="btn-danger" onClick={remove} title="Delete">
            <Trash2 size={15} />
          </button>
        </div>
      </div>

      {!collapsed && (
        <>
          {cell.kind === 'edit' && <EditParamsForm key={cell.updatedAt} cell={cell} onChanged={onChanged} />}
          {cell.kind === 'upload' && <UploadParamsForm cell={cell} editCells={editCells} onChanged={onChanged} />}
          {cell.kind === 'text' && <TextCellForm cell={cell} project={project} onChanged={onChanged} />}

          {cell.kind !== 'upload' && live && cell.status === 'running' && (
            <p class="text-xs text-cyan-400 font-mono">{live}</p>
          )}
          {cell.kind === 'upload' && cell.status === 'running' && (
            <UploadStatusBar info={parseUploadStatus(live)} />
          )}
          {cell.status === 'error' && cell.statusMessage && (
            <p class="text-xs text-red-400 font-mono whitespace-pre-wrap">{cell.statusMessage}</p>
          )}

          {cell.kind === 'edit' && cell.mediaUrl && (
            <>
              <video controls src={cell.mediaUrl} class="w-full rounded-md max-h-80" />
              <MediaInfoBadge cellId={cell.id} status={cell.status} />
              {onAddUpload && (
                <button class="btn-primary w-fit" onClick={onAddUpload}>
                  <Plus size={15} /> Add upload from this edit
                </button>
              )}
            </>
          )}

          {cell.kind === 'upload' && youtubeUrl && <YouTubeLinkBox url={youtubeUrl} />}
        </>
      )}
    </div>
  );
}

// ThumbnailPreview renders a live preview of what the auto-generated
// thumbnail will look like for the title as it's currently typed —
// rendered through the same code path a real run uses, debounced, rather
// than only showing a static image after an actual upload has run.
function ThumbnailPreview({ title }) {
  const [previewUrl, setPreviewUrl] = useState(null);
  const [enabled, setEnabled] = useState(true);
  const [error, setError] = useState('');
  const urlRef = useRef(null);

  useEffect(() => {
    api.getConfig().then((cfg) => setEnabled(cfg.thumbnail?.enabled !== false)).catch(() => {});
  }, []);

  useEffect(() => {
    if (!enabled) return;
    const handle = setTimeout(async () => {
      try {
        const blob = await api.previewThumbnail(title || 'Untitled');
        const url = URL.createObjectURL(blob);
        if (urlRef.current) URL.revokeObjectURL(urlRef.current);
        urlRef.current = url;
        setPreviewUrl(url);
        setError('');
      } catch (e) {
        setError('preview failed: ' + e.message);
      }
    }, 400);
    return () => clearTimeout(handle);
  }, [title, enabled]);

  useEffect(() => () => { if (urlRef.current) URL.revokeObjectURL(urlRef.current); }, []);

  return (
    <div class="space-y-2">
      <div class="flex items-center justify-between">
        <span class="text-xs text-slate-500 flex items-center gap-1">
          <ImageIcon size={13} /> Thumbnail preview
        </span>
        <button
          class="text-xs text-slate-500 hover:text-cyan-400 flex items-center gap-1 transition-colors"
          onClick={() => navigate(paths.config('thumbnail'))}
        >
          <Settings size={12} /> Configure thumbnail
        </button>
      </div>
      {!enabled ? (
        <p class="text-xs text-slate-600">Thumbnail generation is disabled in config.</p>
      ) : previewUrl ? (
        <img src={previewUrl} alt="Live thumbnail preview" class="w-full max-w-md rounded-md border border-slate-800" />
      ) : (
        <p class="text-xs text-slate-600">{error || 'Rendering preview...'}</p>
      )}
    </div>
  );
}

// parseUploadStatus turns the raw SSE log line for an upload cell into a
// structured {label, percent, eta} so the UI can show a real step/progress
// indicator instead of a single opaque line of text.
function parseUploadStatus(msg) {
  if (!msg) return { label: 'Starting...', percent: null, eta: null };
  const uploading = msg.match(/^uploading: ([\d.]+)% \(ETA (.+)\)$/);
  if (uploading) {
    return { label: 'Uploading to YouTube', percent: Number(uploading[1]), eta: uploading[2] };
  }
  if (msg.startsWith('uploaded:')) {
    return { label: 'Upload complete — finalizing', percent: 100, eta: null };
  }
  const processing = msg.match(/processing status: (\w+)/);
  if (processing) {
    return { label: `Processing on YouTube (${processing[1]})`, percent: null, eta: null };
  }
  return { label: msg.replace(/^==>\s*/, ''), percent: null, eta: null };
}

function UploadStatusBar({ info }) {
  return (
    <div class="space-y-1">
      <div class="flex items-center justify-between text-xs text-cyan-400">
        <span class="flex items-center gap-1.5">
          <Loader2 size={12} class="animate-spin" /> {info.label}
        </span>
        {info.eta && <span class="text-slate-500">ETA {info.eta}</span>}
      </div>
      <div class="h-1.5 w-full rounded-full bg-slate-800 overflow-hidden">
        <div
          class={`h-full bg-cyan-500 ${info.percent == null ? 'animate-pulse w-1/3' : 'transition-all'}`}
          style={info.percent != null ? { width: `${info.percent}%` } : undefined}
        />
      </div>
    </div>
  );
}

function YouTubeLinkBox({ url }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // clipboard permission denied or unavailable — the textbox itself
      // is still selectable/copyable by hand.
    }
  };
  return (
    <div class="flex items-center gap-2">
      <input
        class="input flex-1 font-mono text-xs"
        readOnly
        value={url}
        onFocus={(e) => e.currentTarget.select()}
      />
      <button class="btn-secondary" onClick={copy} title="Copy link">
        {copied ? <Check size={15} /> : <Copy size={15} />}
      </button>
      <a href={url} target="_blank" rel="noreferrer" class="btn-secondary" title="Open on YouTube">
        <ExternalLink size={15} />
      </a>
    </div>
  );
}

// GifExportButton toggles a small popover for exporting an edit cell's
// output as an animated GIF, with fps/width customization (both optional
// — blank width keeps the source's own width). The conversion happens
// server-side on click and streams straight back as a blob to save,
// rather than being persisted anywhere.
function GifExportButton({ cell }) {
  const [open, setOpen] = useState(false);
  const [fps, setFps] = useState(12);
  const [width, setWidth] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const boxRef = useRef(null);

  useEffect(() => {
    if (!open) return;
    const onClickAway = (e) => {
      if (!boxRef.current?.contains(e.target)) setOpen(false);
    };
    document.addEventListener('mousedown', onClickAway);
    return () => document.removeEventListener('mousedown', onClickAway);
  }, [open]);

  const exportGif = async () => {
    setBusy(true);
    setError('');
    try {
      const blob = await api.exportGif(cell.id, { fps: Number(fps) || 12, width: Number(width) || 0 });
      await saveBlob(blob, `${sanitizeFilename(cell.name)}.gif`, {
        description: 'GIF image',
        accept: { 'image/gif': ['.gif'] },
      });
      setOpen(false);
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div class="relative" ref={boxRef}>
      <button type="button" class="btn-secondary" onClick={() => setOpen((o) => !o)} title="Export as GIF">
        <FileImage size={15} />
      </button>
      {open && (
        <div class="absolute top-full right-0 mt-1.5 w-56 rounded-md border border-slate-700 bg-slate-950 p-3 text-xs shadow-xl z-10 space-y-2">
          <p class="text-slate-400 font-medium">Export as GIF</p>
          <div class="flex gap-2">
            <div class="flex-1">
              <label class="label">FPS</label>
              <input class="input py-1" type="number" min="1" max="30" value={fps} onInput={(e) => setFps(e.currentTarget.value)} />
            </div>
            <div class="flex-1">
              <label class="label">Width</label>
              <input class="input py-1" type="number" min="1" placeholder="original" value={width} onInput={(e) => setWidth(e.currentTarget.value)} />
            </div>
          </div>
          {error && <p class="text-red-400">{error}</p>}
          <button type="button" class="btn-primary w-full justify-center" disabled={busy} onClick={exportGif}>
            {busy ? <Loader2 size={14} class="animate-spin" /> : <FileImage size={14} />}
            {busy ? 'Rendering...' : 'Download GIF'}
          </button>
        </div>
      )}
    </div>
  );
}

// ProfileMenuButton is an edit cell's "..." overflow menu, holding the two
// profile actions (save the cell's current params as a new named profile;
// apply a saved one). Kept out of the main toolbar row since these are
// occasional actions, not everyday ones like Run/Download.
function ProfileMenuButton({ cell, onChanged }) {
  const [open, setOpen] = useState(false);
  const [view, setView] = useState('menu'); // 'menu' | 'save' | 'apply'
  const boxRef = useRef(null);

  useEffect(() => {
    if (!open) return;
    const onClickAway = (e) => {
      if (!boxRef.current?.contains(e.target)) setOpen(false);
    };
    document.addEventListener('mousedown', onClickAway);
    return () => document.removeEventListener('mousedown', onClickAway);
  }, [open]);

  const close = () => {
    setOpen(false);
    setView('menu');
  };

  return (
    <div class="relative" ref={boxRef}>
      <button
        type="button"
        class="btn-secondary"
        onClick={() => setOpen((o) => !o)}
        title="Profile actions"
        aria-haspopup="menu"
        aria-expanded={open}
      >
        <MoreHorizontal size={15} />
      </button>
      {open && view === 'menu' && (
        <div class="absolute top-full right-0 mt-1.5 w-52 rounded-md border border-slate-700 bg-slate-950 p-1 text-xs shadow-xl z-10">
          <button
            type="button"
            class="w-full flex items-center gap-2 px-2.5 py-1.5 rounded text-slate-300 hover:bg-slate-800 hover:text-cyan-400 transition-colors text-left"
            onClick={() => setView('save')}
          >
            <Save size={14} /> Save as profile
          </button>
          <button
            type="button"
            class="w-full flex items-center gap-2 px-2.5 py-1.5 rounded text-slate-300 hover:bg-slate-800 hover:text-cyan-400 transition-colors text-left"
            onClick={() => setView('apply')}
          >
            <Bookmark size={14} /> Apply profile
          </button>
        </div>
      )}
      {open && view === 'save' && <SaveProfilePopover cell={cell} onBack={() => setView('menu')} onDone={close} />}
      {open && view === 'apply' && (
        <ApplyProfilePopover cell={cell} onChanged={onChanged} onBack={() => setView('menu')} onDone={close} />
      )}
    </div>
  );
}

function PopoverBackHeader({ title, onBack }) {
  return (
    <div class="flex items-center gap-1.5">
      <button type="button" class="text-slate-500 hover:text-slate-300 transition-colors" onClick={onBack} title="Back">
        <ChevronLeft size={14} />
      </button>
      <p class="text-slate-400 font-medium">{title}</p>
    </div>
  );
}

// SaveProfilePopover asks for a name, saves the cell's current params under
// it, and reports success/failure inline.
function SaveProfilePopover({ cell, onBack, onDone }) {
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState(null); // { ok, message }

  const submit = () => {
    const trimmed = name.trim();
    if (!trimmed || busy) return;
    setBusy(true);
    api
      .saveProfile(trimmed, cell.id)
      .then(() => setResult({ ok: true, message: `Saved "${trimmed}".` }))
      .catch((e) => setResult({ ok: false, message: e.message }))
      .finally(() => setBusy(false));
  };

  return (
    <div class="absolute top-full right-0 mt-1.5 w-64 rounded-md border border-slate-700 bg-slate-950 p-3 text-xs shadow-xl z-10 space-y-2">
      <PopoverBackHeader title="Save as profile" onBack={onBack} />
      {result ? (
        <>
          <p class={result.ok ? 'text-emerald-400' : 'text-red-400'}>{result.message}</p>
          <button type="button" class="btn-secondary w-full justify-center" onClick={result.ok ? onDone : () => setResult(null)}>
            {result.ok ? 'Close' : 'Try again'}
          </button>
        </>
      ) : (
        <>
          <input
            class="input py-1"
            placeholder="Profile name"
            value={name}
            autoFocus
            onInput={(e) => setName(e.currentTarget.value)}
            onKeyDown={(e) => e.key === 'Enter' && submit()}
          />
          <button type="button" class="btn-primary w-full justify-center" disabled={busy || !name.trim()} onClick={submit}>
            {busy ? <Loader2 size={14} class="animate-spin" /> : <Save size={14} />}
            {busy ? 'Saving...' : 'Save'}
          </button>
        </>
      )}
    </div>
  );
}

// summarizeProfile renders a profile's params as a compact one-line
// description for the apply-profile list.
function summarizeProfile(p = {}) {
  const parts = [p.scalePct > 0 && p.scalePct < 100 ? `${p.scalePct}%` : 'original size'];
  if (p.speed && p.speed !== 1) parts.push(`${p.speed}x`);
  if (p.margin) parts.push(`margin ${p.margin}`);
  if (p.bitrateKbps) parts.push(`${p.bitrateKbps}kbps`);
  if (p.skipDenoise) parts.push('no denoise');
  return parts.join(' · ');
}

// ApplyProfilePopover lists saved profiles; clicking one applies it to this
// cell (resolving its scale percentage against the cell's actual source
// resolution server-side). Each row also has inline edit/delete so a
// profile can be tweaked without leaving the picker.
function ApplyProfilePopover({ cell, onChanged, onBack, onDone }) {
  const [profiles, setProfiles] = useState(null); // null = loading
  const [editingId, setEditingId] = useState(null);
  const [draft, setDraft] = useState(null);
  const [busyId, setBusyId] = useState(null);
  const [error, setError] = useState('');

  useEffect(() => {
    api
      .listProfiles()
      .then(setProfiles)
      .catch((e) => setError(e.message));
  }, []);

  const startEdit = (p) => {
    setError('');
    setEditingId(p.id);
    setDraft({ name: p.name, ...p.params });
  };
  const cancelEdit = () => {
    setEditingId(null);
    setDraft(null);
  };
  const saveEdit = () => {
    api
      .updateProfile(editingId, {
        name: draft.name,
        params: {
          margin: draft.margin || '0.2s',
          speed: Number(draft.speed) || 1,
          scalePct: Number(draft.scalePct) || 0,
          bitrateKbps: Number(draft.bitrateKbps) || 0,
          lockAspect: !!draft.lockAspect,
          skipDenoise: !!draft.skipDenoise,
        },
      })
      .then((updated) => {
        setProfiles((list) => list.map((p) => (p.id === updated.id ? updated : p)));
        cancelEdit();
      })
      .catch((e) => setError(e.message));
  };
  const deleteProfile = (id) => {
    if (!confirm('Delete this profile?')) return;
    api
      .deleteProfile(id)
      .then(() => setProfiles((list) => list.filter((p) => p.id !== id)))
      .catch((e) => setError(e.message));
  };
  const apply = (id) => {
    setError('');
    setBusyId(id);
    api
      .applyProfile(cell.id, id)
      .then(() => {
        onChanged();
        onDone();
      })
      .catch((e) => {
        setError(e.message);
        setBusyId(null);
      });
  };

  return (
    <div class="absolute top-full right-0 mt-1.5 w-80 rounded-md border border-slate-700 bg-slate-950 p-3 text-xs shadow-xl z-10 space-y-2 max-h-[26rem] overflow-y-auto">
      <PopoverBackHeader title="Apply profile" onBack={onBack} />
      {error && <p class="text-red-400">{error}</p>}
      {!profiles && <p class="text-slate-500">Loading...</p>}
      {profiles && profiles.length === 0 && <p class="text-slate-500">No saved profiles yet — use "Save as profile" first.</p>}
      <div class="space-y-1.5">
        {profiles?.map((p) =>
          editingId === p.id ? (
            <div key={p.id} class="rounded border border-slate-700 p-2 space-y-1.5">
              <input class="input py-1" value={draft.name} onInput={(e) => setDraft({ ...draft, name: e.currentTarget.value })} />
              <div class="grid grid-cols-2 gap-1.5">
                <div>
                  <label class="label">Margin</label>
                  <input class="input py-1" value={draft.margin || ''} onInput={(e) => setDraft({ ...draft, margin: e.currentTarget.value })} />
                </div>
                <div>
                  <label class="label">Speed</label>
                  <input
                    class="input py-1"
                    type="number"
                    step="0.05"
                    value={draft.speed ?? 1}
                    onInput={(e) => setDraft({ ...draft, speed: e.currentTarget.value })}
                  />
                </div>
                <div>
                  <label class="label">Scale %</label>
                  <input
                    class="input py-1"
                    type="number"
                    min="0"
                    max="100"
                    placeholder="original"
                    value={draft.scalePct || ''}
                    onInput={(e) => setDraft({ ...draft, scalePct: e.currentTarget.value })}
                  />
                </div>
                <div>
                  <label class="label">Bitrate</label>
                  <input
                    class="input py-1"
                    type="number"
                    min="0"
                    placeholder="original"
                    value={draft.bitrateKbps || ''}
                    onInput={(e) => setDraft({ ...draft, bitrateKbps: e.currentTarget.value })}
                  />
                </div>
              </div>
              <div class="flex items-center gap-3 text-[11px] text-slate-400">
                <label class="flex items-center gap-1.5">
                  <input
                    type="checkbox"
                    class="accent-cyan-500"
                    checked={!!draft.lockAspect}
                    onChange={(e) => setDraft({ ...draft, lockAspect: e.currentTarget.checked })}
                  />
                  Lock aspect
                </label>
                <label class="flex items-center gap-1.5">
                  <input
                    type="checkbox"
                    class="accent-cyan-500"
                    checked={!!draft.skipDenoise}
                    onChange={(e) => setDraft({ ...draft, skipDenoise: e.currentTarget.checked })}
                  />
                  Skip denoise
                </label>
              </div>
              <div class="flex items-center gap-1.5 pt-0.5">
                <button type="button" class="btn-primary flex-1 justify-center py-1" onClick={saveEdit}>
                  Save
                </button>
                <button type="button" class="btn-secondary flex-1 justify-center py-1" onClick={cancelEdit}>
                  Cancel
                </button>
              </div>
            </div>
          ) : (
            <div key={p.id} class="flex items-center gap-1 rounded hover:bg-slate-900 pl-2 pr-1 py-1">
              <button type="button" class="flex-1 min-w-0 text-left" disabled={busyId === p.id} onClick={() => apply(p.id)}>
                <span class="block truncate text-slate-200">{p.name}</span>
                <span class="block truncate text-[11px] text-slate-500">{summarizeProfile(p.params)}</span>
              </button>
              {busyId === p.id ? (
                <Loader2 size={14} class="animate-spin text-cyan-400 shrink-0 mr-1.5" />
              ) : (
                <>
                  <button type="button" class="text-slate-500 hover:text-cyan-400 p-1 shrink-0" title="Edit" onClick={() => startEdit(p)}>
                    <Pencil size={13} />
                  </button>
                  <button type="button" class="text-slate-500 hover:text-red-400 p-1 shrink-0" title="Delete" onClick={() => deleteProfile(p.id)}>
                    <Trash2 size={13} />
                  </button>
                </>
              )}
            </div>
          )
        )}
      </div>
    </div>
  );
}

// EditParamsForm holds the auto-edit params (margin/speed) plus optional
// resize/bitrate overrides. Resize and bitrate default to blank ("keep
// original" — params.width/height/bitrateKbps of 0 tell the backend not
// to re-encode for that reason at all), with the source video's actual
// values shown as placeholders so "original" isn't a guess. Aspect ratio
// is locked by default: editing one resize dimension recomputes the other
// from the source's ratio; unlocking (the lock icon between the fields)
// lets width/height be set independently.
function EditParamsForm({ cell, onChanged }) {
  const p = cell.params || {};
  const [margin, setMargin] = useState(p.margin || '0.2s');
  const [speed, setSpeed] = useState(p.speed || 1.0);
  const [width, setWidth] = useState(p.width ? String(p.width) : '');
  const [height, setHeight] = useState(p.height ? String(p.height) : '');
  const [bitrate, setBitrate] = useState(p.bitrateKbps ? String(p.bitrateKbps) : '');
  const [lockAspect, setLockAspect] = useState(p.lockAspect !== false);
  const [denoise, setDenoise] = useState(!p.skipDenoise);
  const [info, setInfo] = useState(null);
  const [showMarginHelp, setShowMarginHelp] = useState(false);
  const disabled = cell.status !== 'idle';
  const sourceInfo = info?.source;
  // Prefer this cell's own last output as the size-estimate baseline: it
  // already reflects auto-editor's silence cuts, which the source's raw
  // duration doesn't — so a re-run with tweaked resize/bitrate estimates
  // much closer to reality than starting from the uncut source every time.
  const baseInfo = info?.self || sourceInfo;

  useEffect(() => {
    api.getCellInfo(cell.id).then(setInfo).catch(() => setInfo(null));
  }, [cell.id, cell.status]);

  const save = (overrides = {}) =>
    api
      .updateCell(cell.id, {
        params: {
          margin,
          speed: Number(speed),
          width: Number(width) || 0,
          height: Number(height) || 0,
          bitrateKbps: Number(bitrate) || 0,
          lockAspect,
          skipDenoise: !denoise,
          ...overrides,
        },
      })
      .then(onChanged);

  const toggleDenoise = () => {
    const next = !denoise;
    setDenoise(next);
    save({ skipDenoise: !next });
  };

  const onWidthChange = (v) => {
    setWidth(v);
    if (lockAspect && v && sourceInfo?.width && sourceInfo?.height) {
      setHeight(String(Math.round((Number(v) * sourceInfo.height) / sourceInfo.width)));
    }
  };
  const onHeightChange = (v) => {
    setHeight(v);
    if (lockAspect && v && sourceInfo?.width && sourceInfo?.height) {
      setWidth(String(Math.round((Number(v) * sourceInfo.width) / sourceInfo.height)));
    }
  };
  const toggleLock = () => {
    const next = !lockAspect;
    setLockAspect(next);
    save({ lockAspect: next });
  };

  // applyScale sets width/height to pct% of the source's own resolution
  // (100% clears the override back to "keep original") — a quicker path
  // than typing exact pixel dimensions.
  const applyScale = (pct) => {
    if (!sourceInfo?.width || !sourceInfo?.height || !pct) return;
    if (pct >= 100) {
      setWidth('');
      setHeight('');
      save({ width: 0, height: 0 });
      return;
    }
    const w = Math.round((sourceInfo.width * pct) / 100);
    const h = Math.round((sourceInfo.height * pct) / 100);
    setWidth(String(w));
    setHeight(String(h));
    save({ width: w, height: h });
  };

  const estimatedBytes = estimateOutputBytes({
    baseInfo,
    width: Number(width) || 0,
    height: Number(height) || 0,
    bitrateKbps: Number(bitrate) || 0,
  });

  return (
    <div class="space-y-4">
      <div class="form-section">
        <h4 class="form-section-title"><Scissors size={12} /> Trim &amp; speed</h4>
        <div class="flex gap-4">
          <div class="flex-1">
            <label class="label flex items-center gap-1">
              Margin
              <button
                type="button"
                class="text-slate-500 hover:text-cyan-400 transition-colors"
                onClick={() => setShowMarginHelp((s) => !s)}
                title="What does margin do?"
              >
                <CircleQuestionMark size={12} />
              </button>
            </label>
            <input class="input" value={margin} disabled={disabled} onInput={(e) => setMargin(e.currentTarget.value)} onBlur={() => save()} />
            {showMarginHelp && (
              <p class="text-[11px] text-slate-500 mt-1 leading-snug">
                How much extra time to keep on either side of detected speech before cutting, so words
                don't get clipped at the start/end of a cut. <code class="text-slate-400">0.2s</code>{' '}
                (default) is a light trim; raise it (e.g. <code class="text-slate-400">0.3s</code>–
                <code class="text-slate-400">0.5s</code>) if cuts feel abrupt, lower it for a tighter edit.
              </p>
            )}
          </div>
          <div class="flex-1">
            <label class="label">Speed</label>
            <input
              class="input"
              type="number"
              step="0.05"
              min="0.5"
              max="4"
              value={speed}
              disabled={disabled}
              onInput={(e) => setSpeed(e.currentTarget.value)}
              onBlur={() => save()}
            />
            <div class="flex items-center gap-1 mt-1.5">
              {[1, 1.25, 1.5, 1.75, 2].map((preset) => (
                <button
                  key={preset}
                  type="button"
                  class={`text-[11px] px-2 py-0.5 rounded border transition-colors disabled:opacity-40 disabled:cursor-not-allowed ${
                    Number(speed) === preset
                      ? 'border-cyan-600 bg-cyan-950 text-cyan-300'
                      : 'border-slate-700 text-slate-400 hover:text-cyan-400 hover:border-cyan-700'
                  }`}
                  disabled={disabled}
                  onClick={() => {
                    setSpeed(preset);
                    save({ speed: preset });
                  }}
                >
                  {preset}x
                </button>
              ))}
            </div>
          </div>
        </div>
      </div>

      <div class="form-section">
        <h4 class="form-section-title"><Wind size={12} /> Audio</h4>
        <label class="flex items-start justify-between gap-3">
          <span>
            <span class="block text-sm text-slate-200">Denoise with DeepFilterNet</span>
            <span class="block text-xs text-slate-500 mt-0.5">
              Cleans background noise/hiss before cutting silence. Turn off for already-clean audio to skip the slowest stage.
            </span>
          </span>
          <button
            type="button"
            role="switch"
            aria-checked={denoise}
            disabled={disabled}
            onClick={toggleDenoise}
            class="switch mt-0.5"
          >
            <span class="switch-thumb" />
          </button>
        </label>
      </div>

      <div class="form-section">
        <h4 class="form-section-title"><Maximize2 size={12} /> Resize</h4>
        <div class="flex items-center gap-2">
          <input
            class="input flex-1"
            type="number"
            min="1"
            placeholder={sourceInfo ? `${sourceInfo.width} (original)` : 'width'}
            value={width}
            disabled={disabled}
            onInput={(e) => onWidthChange(e.currentTarget.value)}
            onBlur={() => save()}
          />
          <button
            type="button"
            class={`shrink-0 inline-flex items-center gap-1 rounded-md px-2.5 py-1.5 text-xs font-medium border transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${
              lockAspect
                ? 'bg-cyan-600 border-cyan-600 hover:bg-cyan-500 text-white'
                : 'bg-slate-900 border-slate-700 hover:bg-slate-800 text-slate-500'
            }`}
            disabled={disabled}
            aria-pressed={lockAspect}
            onClick={toggleLock}
            title={lockAspect ? 'Aspect ratio locked — click to unlock' : 'Aspect ratio unlocked — click to lock'}
          >
            {lockAspect ? <Lock size={15} /> : <LockOpen size={15} />}
          </button>
          <input
            class="input flex-1"
            type="number"
            min="1"
            placeholder={sourceInfo ? `${sourceInfo.height} (original)` : 'height'}
            value={height}
            disabled={disabled}
            onInput={(e) => onHeightChange(e.currentTarget.value)}
            onBlur={() => save()}
          />
        </div>
        <div class="flex items-center gap-1.5">
          <span class="text-[11px] text-slate-500 mr-0.5">Scale:</span>
          {[25, 50, 75, 100].map((pct) => (
            <button
              key={pct}
              type="button"
              class="text-[11px] px-2 py-0.5 rounded border border-slate-700 text-slate-400 hover:text-cyan-400 hover:border-cyan-700 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
              disabled={disabled || !sourceInfo}
              onClick={() => applyScale(pct)}
              title={pct === 100 ? 'Reset to original resolution' : `Scale to ${pct}% of original`}
            >
              {pct === 100 ? 'Original' : `${pct}%`}
            </button>
          ))}
          <input
            class="input py-0.5 text-[11px] w-16"
            type="number"
            min="1"
            max="100"
            placeholder="%"
            disabled={disabled || !sourceInfo}
            onKeyDown={(e) => {
              if (e.key !== 'Enter') return;
              applyScale(Number(e.currentTarget.value));
              e.currentTarget.value = '';
            }}
            title="Custom scale % of original — press Enter to apply"
          />
        </div>
      </div>

      <div class="form-section">
        <h4 class="form-section-title"><Gauge size={12} /> Bitrate</h4>
        <input
          class="input"
          type="number"
          min="1"
          placeholder={sourceInfo?.bitrateKbps ? `${sourceInfo.bitrateKbps} kbps (original)` : 'kbps, blank = original'}
          value={bitrate}
          disabled={disabled}
          onInput={(e) => setBitrate(e.currentTarget.value)}
          onBlur={() => save()}
        />
      </div>

      {!disabled && estimatedBytes != null && (
        <p class="text-xs text-slate-500 border-t border-slate-800/80 pt-3" title="Approximate — based on the current margin/speed/resize/bitrate settings and this source's typical bitrate; actual size depends on scene complexity and how much silence auto-editor cuts.">
          Estimated output: ~{formatSize(estimatedBytes)} (approx)
        </p>
      )}
    </div>
  );
}

// estimateOutputBytes gives a rough pre-run size estimate. It doesn't know
// how much auto-editor will cut, so baseInfo's own duration is the best
// available guess for the output's length — already-cut if this cell has
// run before, otherwise the uncut source's.
//
// Without an explicit bitrate override, it scales baseInfo's own bitrate
// by the requested resize's pixel-area ratio (CRF-based encodes scale
// roughly that way). Scaling off baseInfo rather than always off the
// uncut source matters: once this cell has run once, baseInfo IS that
// prior output, whose bitrate already reflects this exact footage's real
// compressibility at roughly the requested settings — a far better
// reference than the source's very differently-encoded bitrate, which
// this content may compress nothing like (e.g. a static screen recording
// compresses much harder than its source bitrate implies).
function estimateOutputBytes({ baseInfo, width, height, bitrateKbps }) {
  if (!baseInfo || !baseInfo.durationSec) return null;

  let videoKbps = bitrateKbps;
  if (!videoKbps) {
    videoKbps = baseInfo.bitrateKbps || 0;
    if (width && height && baseInfo.width && baseInfo.height) {
      videoKbps *= (width * height) / (baseInfo.width * baseInfo.height);
    }
  }
  if (!videoKbps) return null;

  const audioKbps = baseInfo.audioBitrateKbps || 160;
  return ((videoKbps + audioKbps) * 1000 * baseInfo.durationSec) / 8;
}

// TextCellForm is a markdown note: it always shows the rendered view once
// there's content, with a single Edit button to go change it — no
// separate preview tab, since flipping back and forth to check formatting
// isn't the point here. Saving (via Done) returns straight to the
// rendered view. While editing, "Insert reference" drops in an @<seq>
// mention for any other cell in the project (source/edit/upload/text),
// which renders as a link to that cell.
function TextCellForm({ cell, project, onChanged }) {
  const p = cell.params || {};
  const [markdown, setMarkdown] = useState(p.markdown || '');
  const [mode, setMode] = useState(p.markdown ? 'view' : 'edit');
  const [pickerOpen, setPickerOpen] = useState(false);
  const textareaRef = useRef(null);
  const pickerRef = useRef(null);

  useEffect(() => {
    setMarkdown(p.markdown || '');
    setMode(p.markdown ? 'view' : 'edit');
  }, [cell.id]);

  useEffect(() => {
    if (!pickerOpen) return;
    const onClickAway = (e) => {
      if (!pickerRef.current?.contains(e.target)) setPickerOpen(false);
    };
    document.addEventListener('mousedown', onClickAway);
    return () => document.removeEventListener('mousedown', onClickAway);
  }, [pickerOpen]);

  const save = () => {
    if (markdown === (p.markdown || '')) return Promise.resolve();
    return api.updateCell(cell.id, { params: { markdown } }).then(onChanged);
  };
  const finishEditing = async () => {
    await save();
    setMode('view');
  };

  const insertReference = (target) => {
    const token = `@${target.seq}`;
    const el = textareaRef.current;
    const start = el?.selectionStart ?? markdown.length;
    const end = el?.selectionEnd ?? markdown.length;
    const next = markdown.slice(0, start) + token + ' ' + markdown.slice(end);
    setMarkdown(next);
    setPickerOpen(false);
    requestAnimationFrame(() => {
      if (!el) return;
      el.focus();
      const pos = start + token.length + 1;
      el.setSelectionRange(pos, pos);
    });
  };

  const otherCells = (project?.cells || []).filter((c) => c.id !== cell.id);

  if (mode === 'view') {
    return (
      <div class="space-y-2">
        <div class="flex justify-end">
          <button type="button" class="btn-secondary" onClick={() => setMode('edit')}>
            <Pencil size={13} /> Edit
          </button>
        </div>
        {markdown ? (
          <div
            class="markdown-body rounded-md border border-slate-800 bg-slate-950 px-3 py-2"
            dangerouslySetInnerHTML={{ __html: renderMarkdown(markdown, project) }}
          />
        ) : (
          <p class="text-xs text-slate-600 flex items-center gap-1.5">
            <FileText size={13} /> Nothing here yet.
          </p>
        )}
      </div>
    );
  }

  return (
    <div class="space-y-2">
      <div class="flex items-center gap-2">
        <div class="relative" ref={pickerRef}>
          <button type="button" class="btn-secondary" onClick={() => setPickerOpen((o) => !o)}>
            <Link2 size={13} /> Insert reference
          </button>
          {pickerOpen && (
            <div class="absolute top-full left-0 mt-1.5 w-64 max-h-56 overflow-y-auto rounded-md border border-slate-700 bg-slate-950 p-1 text-xs shadow-xl z-10">
              {otherCells.length === 0 && <p class="px-2 py-1.5 text-slate-500">No other cells in this project yet.</p>}
              {otherCells.map((c) => (
                <button
                  key={c.id}
                  type="button"
                  class="w-full text-left px-2 py-1.5 rounded hover:bg-slate-800 flex items-center justify-between gap-2"
                  onClick={() => insertReference(c)}
                >
                  <span class="truncate">{c.name}</span>
                  <span class="text-slate-500 shrink-0">#{c.seq} · {c.kind}</span>
                </button>
              ))}
            </div>
          )}
        </div>
        <button type="button" class="btn-primary ml-auto" onClick={finishEditing}>
          <Check size={14} /> Done
        </button>
      </div>

      <textarea
        ref={textareaRef}
        class="input font-mono text-xs min-h-[10rem] resize-y"
        placeholder="Links, timestamps, notes... (markdown supported, @seq to reference a cell)"
        value={markdown}
        onInput={(e) => setMarkdown(e.currentTarget.value)}
        onBlur={save}
        autoFocus
      />
    </div>
  );
}

function UploadParamsForm({ cell, editCells, onChanged }) {
  const p = cell.params || {};
  const [title, setTitle] = useState(p.title || '');
  const [privacy, setPrivacy] = useState(p.privacy || 'unlisted');
  const [tags, setTags] = useState((p.tags || []).join(', '));
  const disabled = cell.status !== 'idle';

  const save = () =>
    api
      .updateCell(cell.id, {
        params: { ...p, title, privacy, tags: tags.split(',').map((t) => t.trim()).filter(Boolean) },
      })
      .then(onChanged);

  const sourceEdit = editCells.find((c) => c.id === cell.parentCellId);

  return (
    <div class="space-y-3">
      <p class="text-xs text-slate-500">Uploads: {sourceEdit ? sourceEdit.name : 'unknown edit cell'}</p>
      <div>
        <label class="label">Title</label>
        <input class="input" value={title} disabled={disabled} onInput={(e) => setTitle(e.currentTarget.value)} onBlur={save} />
      </div>
      <div class="flex gap-4">
        <div class="flex-1">
          <label class="label">Privacy</label>
          <select class="input" value={privacy} disabled={disabled} onChange={(e) => { setPrivacy(e.currentTarget.value); save(); }}>
            <option value="unlisted">Unlisted</option>
            <option value="public">Public</option>
            <option value="private">Private</option>
          </select>
        </div>
        <div class="flex-1">
          <label class="label">Extra tags (comma-separated)</label>
          <input class="input" value={tags} disabled={disabled} onInput={(e) => setTags(e.currentTarget.value)} onBlur={save} />
        </div>
      </div>
      <ThumbnailPreview title={title} />
    </div>
  );
}
