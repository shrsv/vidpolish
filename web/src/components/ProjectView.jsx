import { useEffect, useState } from 'preact/hooks';
import { ArrowLeft, Plus } from 'lucide-preact';
import { api } from '../api.js';
import { SourceDropzone } from './SourceDropzone.jsx';
import { Cell } from './Cell.jsx';

export function ProjectView({ projectId, onBack }) {
  const [project, setProject] = useState(null);
  const [error, setError] = useState('');

  const refresh = () => api.getProject(projectId).then(setProject).catch((e) => setError(e.message));
  useEffect(refresh, [projectId]);

  if (error) return <p class="text-sm text-red-400">{error}</p>;
  if (!project) return <p class="text-sm text-slate-500">Loading...</p>;

  const source = project.cells.find((c) => c.kind === 'source');
  const editCells = project.cells.filter((c) => c.kind === 'edit');
  const uploadCells = project.cells.filter((c) => c.kind === 'upload');

  const addEdit = async () => {
    await api.createCell(projectId, {
      kind: 'edit',
      parentCellId: source.id,
      params: { margin: '0.2s', speed: 1.0 },
    });
    refresh();
  };

  const addUpload = async (editCellId) => {
    await api.createCell(projectId, {
      kind: 'upload',
      parentCellId: editCellId,
      params: { title: '', privacy: 'unlisted', tags: [] },
    });
    refresh();
  };

  return (
    <div class="space-y-6">
      <div class="flex items-center gap-3">
        <button class="btn-secondary" onClick={onBack}>
          <ArrowLeft size={15} /> Projects
        </button>
        <h1 class="text-lg font-semibold">{project.name}</h1>
      </div>

      {/* Source cell */}
      {source.mediaUrl ? (
        <div class="card p-4 space-y-2">
          <div class="flex items-center gap-2">
            <span class="text-xs font-mono text-slate-500">#{source.seq}</span>
            <span class="font-medium">{source.name}</span>
            <span class="text-xs text-slate-500">{source.sourceFilename}</span>
          </div>
          <video controls src={source.mediaUrl} class="w-full rounded-md max-h-80" />
        </div>
      ) : (
        <SourceDropzone cellId={source.id} onDone={refresh} />
      )}

      {source.mediaUrl && (
        <>
          <div class="flex items-center justify-between">
            <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wide">Edit cells</h2>
            <button class="btn-secondary" onClick={addEdit}>
              <Plus size={15} /> Add edit cell
            </button>
          </div>
          <div class="space-y-4">
            {editCells.map((c) => (
              <div key={c.id} class="space-y-2">
                <Cell cell={c} editCells={editCells} onChanged={refresh} onDelete={refresh} />
                {c.mediaUrl && (
                  <button class="btn-secondary ml-4" onClick={() => addUpload(c.id)}>
                    <Plus size={13} /> Add upload from this edit
                  </button>
                )}
              </div>
            ))}
            {editCells.length === 0 && (
              <p class="text-sm text-slate-500">No edit cells yet. Add one to denoise/cut at a chosen margin and speed.</p>
            )}
          </div>
        </>
      )}

      {uploadCells.length > 0 && (
        <>
          <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wide">Upload cells</h2>
          <div class="space-y-4">
            {uploadCells.map((c) => (
              <Cell key={c.id} cell={c} editCells={editCells} onChanged={refresh} onDelete={refresh} />
            ))}
          </div>
        </>
      )}
    </div>
  );
}
