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
} from 'lucide-preact';
import { api } from '../api.js';
import { navigate, paths } from '../router.js';

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
    return () => stopRef.current?.();
  }, [cell.status, cell.id]);

  const run = () => api.runCell(cell.id).then(onChanged);
  const remove = () => {
    if (confirm(`Delete "${cell.name}"?`)) api.deleteCell(cell.id).then(onDelete);
  };
  const download = async () => {
    const suggestedName = `${cell.name.replace(/\s+/g, '-')}.mp4`;
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
          <span class={STATUS_CLASS[cell.status]}>{cell.status}</span>
          {parent && (
            <button
              class="text-xs text-slate-500 hover:text-cyan-400 flex items-center gap-1 transition-colors shrink-0"
              onClick={() => navigate(paths.cell(project.id, parent.seq))}
              title={`Based on #${parent.seq} ${parent.name}`}
            >
              <Link2 size={12} /> based on #{parent.seq}
            </button>
          )}
        </div>

        {/* Cell toolbar: one disciplined place for every action. */}
        <div class="flex items-center gap-2 shrink-0">
          <button class="btn-primary" disabled={cell.status === 'running'} onClick={run}>
            {cell.status === 'running' ? <Loader2 size={15} class="animate-spin" /> : <Play size={15} />}
            Run
          </button>
          {cell.kind === 'edit' && cell.mediaUrl && (
            <button class="btn-secondary" onClick={download} title="Download">
              <Download size={15} />
            </button>
          )}
          <button class="btn-danger" onClick={remove} title="Delete">
            <Trash2 size={15} />
          </button>
        </div>
      </div>

      {!collapsed && (
        <>
          {cell.kind === 'edit' && <EditParamsForm cell={cell} onChanged={onChanged} />}
          {cell.kind === 'upload' && <UploadParamsForm cell={cell} editCells={editCells} onChanged={onChanged} />}

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

function EditParamsForm({ cell, onChanged }) {
  const [margin, setMargin] = useState(cell.params?.margin || '0.2s');
  const [speed, setSpeed] = useState(cell.params?.speed || 1.0);
  const disabled = cell.status !== 'idle';

  const save = () => api.updateCell(cell.id, { params: { margin, speed: Number(speed) } }).then(onChanged);

  return (
    <div class="flex gap-4">
      <div class="flex-1">
        <label class="label">Margin</label>
        <input class="input" value={margin} disabled={disabled} onInput={(e) => setMargin(e.currentTarget.value)} onBlur={save} />
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
          onBlur={save}
        />
      </div>
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
