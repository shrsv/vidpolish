import { useEffect, useState } from 'preact/hooks';
import { Trash2, Sparkles } from 'lucide-preact';
import { api } from '../api.js';

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
      {entries.length === 0 && <p class="text-sm text-slate-500">Cache is empty.</p>}
      {entries.map((e) => (
        <div key={e.fingerprint} class="card p-3 flex items-center justify-between">
          <div>
            <div class="font-mono text-sm">{e.fingerprint}</div>
            <div class="text-xs text-slate-500">
              {formatBytes(e.sizeBytes)}
              {e.cachedAt ? ` · cached ${new Date(e.cachedAt * 1000).toLocaleString()}` : ''}
            </div>
          </div>
          <button class="btn-danger" onClick={() => remove(e.fingerprint)}>
            <Trash2 size={15} />
          </button>
        </div>
      ))}
    </div>
  );
}
