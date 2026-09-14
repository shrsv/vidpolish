import { useEffect, useState } from 'preact/hooks';
import { Plus, ChevronRight, Trash2, Database } from 'lucide-preact';
import { api } from '../api.js';
import { navigate, paths } from '../router.js';
import { timeAgo, fullTimestamp } from '../time.js';

export function ProjectList() {
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
      navigate(paths.project(p.id));
    } catch (e) {
      setError(e.message);
    }
  };

  const remove = async (e, id) => {
    e.stopPropagation();
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
        {projects.map((p) => {
          const hasSource = p.cells.some((c) => c.kind === 'source' && c.mediaUrl);
          return (
            <div
              key={p.id}
              role="button"
              tabIndex={0}
              class="card group flex items-center justify-between px-4 py-3 cursor-pointer transition-colors hover:bg-slate-800/40 hover:border-slate-700"
              onClick={() => navigate(paths.project(p.id))}
              onKeyDown={(e) => e.key === 'Enter' && navigate(paths.project(p.id))}
            >
              <div>
                <div class="font-medium">{p.name}</div>
                <div class="text-xs text-slate-500 flex items-center gap-1.5">
                  <span>{p.cells.length} cell{p.cells.length === 1 ? '' : 's'}</span>
                  <span>·</span>
                  <span
                    title={`Created ${fullTimestamp(p.createdAt)}\nUpdated ${fullTimestamp(p.updatedAt)}`}
                  >
                    Updated {timeAgo(p.updatedAt)}
                  </span>
                </div>
              </div>
              <div class="flex items-center gap-3">
                {hasSource && (
                  <button
                    class="text-xs text-slate-500 hover:text-cyan-400 flex items-center gap-1 transition-colors"
                    onClick={(e) => {
                      e.stopPropagation();
                      navigate(paths.cache());
                    }}
                    title="View cache usage"
                  >
                    <Database size={13} /> Cache
                  </button>
                )}
                <button class="btn-danger" onClick={(e) => remove(e, p.id)} title="Delete project">
                  <Trash2 size={15} />
                </button>
                <ChevronRight size={18} class="text-slate-600 group-hover:text-slate-300 transition-colors" />
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
