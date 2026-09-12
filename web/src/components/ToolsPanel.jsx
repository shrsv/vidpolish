import { useEffect, useState } from 'preact/hooks';
import { CheckCircle2, XCircle, RefreshCw } from 'lucide-preact';
import { api } from '../api.js';

export function ToolsPanel() {
  const [tools, setTools] = useState([]);
  const [busy, setBusy] = useState('');

  const refresh = () => api.listTools().then(setTools);
  useEffect(refresh, []);

  const resolve = async (name) => {
    setBusy(name);
    try {
      await api.resolveTool(name);
    } finally {
      setBusy('');
      refresh();
    }
  };

  return (
    <div class="space-y-2 max-w-2xl">
      <div class="flex items-center justify-between">
        <h2 class="font-semibold">Tool status</h2>
        <button class="btn-secondary" onClick={refresh}>
          <RefreshCw size={15} /> Refresh
        </button>
      </div>
      {tools.map((t) => (
        <div key={t.name} class="card p-3 space-y-1">
          <div class="flex items-center justify-between gap-3">
            <div class="flex items-center gap-2 min-w-0 shrink-0">
              {t.error ? <XCircle size={16} class="text-red-400 shrink-0" /> : <CheckCircle2 size={16} class="text-emerald-400 shrink-0" />}
              <span class="font-medium whitespace-nowrap">{t.name}</span>
            </div>
            <button class="btn-secondary shrink-0" disabled={busy === t.name} onClick={() => resolve(t.name)}>
              {busy === t.name ? 'Resolving...' : 'Re-check'}
            </button>
          </div>
          <div class="text-xs truncate" title={t.error || t.path}>
            {t.error ? <span class="text-red-400">{t.error}</span> : <span class="text-slate-400 font-mono">{t.path}</span>}
          </div>
        </div>
      ))}
    </div>
  );
}
