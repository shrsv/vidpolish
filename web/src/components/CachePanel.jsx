import { useEffect, useState } from 'preact/hooks';
import { Trash2, Sparkles, Info, FolderOpen } from 'lucide-preact';
import { api } from '../api.js';
import { navigate, paths } from '../router.js';

function formatBytes(n) {
  if (n < 1024) return `${n} B`;
  const units = ['KB', 'MB', 'GB'];
  let i = -1;
  do {
    n /= 1024;
    i++;
  } while (n >= 1024 && i < units.length - 1);
  return `${n.toFixed(1)} ${units[i]}`;
}

export function CachePanel() {
  const [entries, setEntries] = useState([]);

  const refresh = () => api.listCache().then(setEntries);
  useEffect(refresh, []);

  const remove = async (fp) => {
    await api.deleteCacheEntry(fp);
    refresh();
  };
  const clean = async () => {
    await api.cleanCache();
    refresh();
  };

  const total = entries.reduce((sum, e) => sum + e.sizeBytes, 0);

  return (
    <div class="space-y-3 max-w-2xl">
      <div class="flex items-center justify-between">
        <h2 class="font-semibold">
          Pipeline cache <span class="text-slate-500 font-normal">({formatBytes(total)} total)</span>
        </h2>
        <button class="btn-secondary" onClick={clean}>
          <Sparkles size={15} /> Clean expired
        </button>
      </div>
      <p class="text-xs text-slate-500 flex gap-1.5 items-start">
        <Info size={14} class="shrink-0 mt-0.5" />
        Cache entries hold shared denoise/split work, keyed by video content — not project data.
        Deleting an entry never deletes any project; it just means the next run re-does that work.
      </p>
      {entries.length === 0 && <p class="text-sm text-slate-500">Cache is empty.</p>}
      {entries.map((e) => (
        <div key={e.fingerprint} class="card p-3 flex items-center justify-between transition-colors hover:border-slate-700">
          <div>
            <div class="font-mono text-sm">{e.fingerprint}</div>
            <div class="text-xs text-slate-500">
              {formatBytes(e.sizeBytes)}
              {e.cachedAt ? ` · cached ${new Date(e.cachedAt * 1000).toLocaleString()}` : ''}
            </div>
            {e.projectName ? (
              <button
                class="text-xs text-cyan-400 hover:text-cyan-300 flex items-center gap-1 mt-0.5"
                onClick={() => navigate(paths.project(e.projectId))}
              >
                <FolderOpen size={12} /> {e.projectName}
              </button>
            ) : (
              <div class="text-xs text-slate-600 mt-0.5">not linked to a current project</div>
            )}
          </div>
          <button class="btn-danger" onClick={() => remove(e.fingerprint)}>
            <Trash2 size={15} />
          </button>
        </div>
      ))}
    </div>
  );
}
