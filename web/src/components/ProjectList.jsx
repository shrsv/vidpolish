import { useEffect, useState } from 'preact/hooks';
import { Plus, ChevronRight, Trash2 } from 'lucide-preact';
import { api } from '../api.js';

export function ProjectList({ onOpen }) {
  const [projects, setProjects] = useState([]);
  const [name, setName] = useState('');
  const [error, setError] = useState('');

  const refresh = () => api.listProjects().then(setProjects).catch((e) => setError(e.message));
  useEffect(refresh, []);

  const create = async (e) => {
    e.preventDefault();
    if (!name.trim()) return;
    try {
      const p = await api.createProject(name.trim());
      setName('');
      await refresh();
      onOpen(p.id);
    } catch (e) {
      setError(e.message);
    }
  };

  const remove = async (id) => {
    if (!confirm('Delete this project and all its cells?')) return;
    await api.deleteProject(id);
    refresh();
  };

  return (
    <div class="space-y-6">
      <form onSubmit={create} class="flex gap-2">
        <input
          class="input flex-1"
          placeholder="New project name..."
          value={name}
          onInput={(e) => setName(e.currentTarget.value)}
        />
        <button class="btn-primary" type="submit">
          <Plus size={15} /> Create
        </button>
      </form>
      {error && <p class="text-sm text-red-400">{error}</p>}

      <div class="space-y-2">
        {projects.length === 0 && (
          <p class="text-sm text-slate-500">No projects yet. Create one above to drop in a video.</p>
        )}
        {projects.map((p) => (
          <div key={p.id} class="card flex items-center justify-between px-4 py-3">
            <button class="flex-1 text-left" onClick={() => onOpen(p.id)}>
              <div class="font-medium">{p.name}</div>
              <div class="text-xs text-slate-500">{p.cells.length} cell{p.cells.length === 1 ? '' : 's'}</div>
            </button>
            <div class="flex items-center gap-2">
              <button class="btn-secondary" onClick={() => onOpen(p.id)}>
                Open <ChevronRight size={15} />
              </button>
              <button class="btn-danger" onClick={() => remove(p.id)}>
                <Trash2 size={15} />
              </button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
