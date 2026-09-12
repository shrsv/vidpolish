import { useEffect, useRef, useState } from 'preact/hooks';
import { Play, Trash2, Pencil, ExternalLink, Loader2 } from 'lucide-preact';
import { api } from '../api.js';

const STATUS_CLASS = {
  idle: 'badge-idle',
  running: 'badge-running',
  done: 'badge-done',
  error: 'badge-error',
};

export function Cell({ cell, editCells, onChanged, onDelete }) {
  const [live, setLive] = useState(cell.statusMessage || '');
  const [editingName, setEditingName] = useState(false);
  const [name, setName] = useState(cell.name);
  const stopRef = useRef(null);

  useEffect(() => {
    if (cell.status !== 'running') return;
    stopRef.current?.();
    stopRef.current = api.subscribeCellEvents(cell.id, (msg) => {
      if (msg === '__done__') {
        onChanged();
        return;
      }
      setLive(msg);
    });
    return () => stopRef.current?.();
  }, [cell.status, cell.id]);

  const run = () => api.runCell(cell.id).then(onChanged);
  const remove = () => {
    if (confirm(`Delete "${cell.name}"?`)) api.deleteCell(cell.id).then(onDelete);
  };
  const saveName = async () => {
    setEditingName(false);
    if (name !== cell.name) await api.updateCell(cell.id, { name });
    onChanged();
  };

  return (
    <div class="card p-4 space-y-3">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-2">
          <span class="text-xs font-mono text-slate-500">#{cell.seq}</span>
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
            <button class="font-medium flex items-center gap-1 hover:text-cyan-400" onClick={() => setEditingName(true)}>
              {cell.name} <Pencil size={12} class="opacity-50" />
            </button>
          )}
          <span class={STATUS_CLASS[cell.status]}>{cell.status}</span>
        </div>
        <div class="flex items-center gap-2">
          <button class="btn-primary" disabled={cell.status === 'running'} onClick={run}>
            {cell.status === 'running' ? <Loader2 size={15} class="animate-spin" /> : <Play size={15} />}
            Run
          </button>
          <button class="btn-danger" onClick={remove}>
            <Trash2 size={15} />
          </button>
        </div>
      </div>

      {cell.kind === 'edit' && <EditParamsForm cell={cell} onChanged={onChanged} />}
      {cell.kind === 'upload' && <UploadParamsForm cell={cell} editCells={editCells} onChanged={onChanged} />}

      {live && cell.status === 'running' && <p class="text-xs text-cyan-400 font-mono">{live}</p>}
      {cell.status === 'error' && cell.statusMessage && (
        <p class="text-xs text-red-400 font-mono whitespace-pre-wrap">{cell.statusMessage}</p>
      )}

      {cell.kind === 'edit' && cell.mediaUrl && (
        <video controls src={cell.mediaUrl} class="w-full rounded-md max-h-80" />
      )}
      {cell.kind === 'upload' && cell.youtubeUrl && (
        <a href={cell.youtubeUrl} target="_blank" rel="noreferrer" class="btn-secondary w-fit">
          <ExternalLink size={15} /> {cell.youtubeUrl}
        </a>
      )}
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
    </div>
  );
}
