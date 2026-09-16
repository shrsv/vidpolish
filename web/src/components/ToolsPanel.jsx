import { useEffect, useRef, useState } from 'preact/hooks';
import { CheckCircle2, XCircle, RefreshCw, DownloadCloud } from 'lucide-preact';
import { api } from '../api.js';

function formatBytes(n) {
  if (!n) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return `${n.toFixed(1)} ${units[i]}`;
}

// Tool status lives entirely on the server (see GET /api/tools): this
// panel just polls it. That's what makes it correct after navigating away
// and back - remounting just asks the server "what's happening right
// now" instead of relying on state (or an event subscription) held only
// in this component, which a tab switch would otherwise throw away.
export function ToolsPanel() {
  const [tools, setTools] = useState([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState('');
  const [rowErrors, setRowErrors] = useState({});
  const [allBusy, setAllBusy] = useState(false);
  const pollTimer = useRef(null);
  const mounted = useRef(true);

  const pollOnce = () => {
    if (pollTimer.current) {
      clearTimeout(pollTimer.current);
      pollTimer.current = null;
    }
    return api
      .listTools()
      .then((data) => {
        if (!mounted.current) return;
        setTools(data);
        setLoadError('');
        if (data.some((t) => t.downloading)) {
          pollTimer.current = setTimeout(pollOnce, 1000);
        }
      })
      .catch((err) => {
        if (mounted.current) setLoadError(err.message || String(err));
      })
      .finally(() => {
        if (mounted.current) setLoading(false);
      });
  };

  useEffect(() => {
    mounted.current = true;
    setLoading(true);
    pollOnce();
    return () => {
      mounted.current = false;
      if (pollTimer.current) clearTimeout(pollTimer.current);
    };
  }, []);

  // "Download" (missing tool) and "Redownload" (present tool, force a
  // fresh fetch even though it's already cached - e.g. to recover from a
  // corrupted download) both just start/force a resolve on the server,
  // then immediately poll to pick up the resulting state.
  const start = (name, redownload) => {
    setRowErrors((prev) => ({ ...prev, [name]: '' }));
    const call = redownload ? api.redownloadTool(name) : api.resolveTool(name);
    call
      .catch((err) => {
        const msg = err.message || String(err);
        if (!msg.includes('already being resolved')) {
          setRowErrors((prev) => ({ ...prev, [name]: msg }));
        }
      })
      .finally(pollOnce);
  };

  const downloadAllMissing = () => {
    setAllBusy(true);
    api
      .resolveAllTools()
      .catch((err) => setLoadError(err.message || String(err)))
      .finally(() => {
        setAllBusy(false);
        pollOnce();
      });
  };

  const missingCount = tools.filter((t) => t.error && !t.downloading).length;

  return (
    <div class="space-y-2 max-w-2xl">
      <div class="flex items-center justify-between">
        <h2 class="font-semibold">Tool status</h2>
        <div class="flex items-center gap-2">
          {missingCount > 0 && (
            <button class="btn-secondary" disabled={allBusy} onClick={downloadAllMissing}>
              <DownloadCloud size={15} /> {allBusy ? 'Starting...' : `Download All Missing (${missingCount})`}
            </button>
          )}
          <button class="btn-secondary" disabled={loading} onClick={pollOnce}>
            <RefreshCw size={15} /> {loading ? 'Checking...' : 'Re-check'}
          </button>
        </div>
      </div>
      {loadError && (
        <div class="card p-3 text-sm text-red-400">Failed to load tool status: {loadError}</div>
      )}
      {!loading && !loadError && tools.length === 0 && (
        <div class="text-sm text-slate-400">No tools reported.</div>
      )}
      {tools.map((t) => {
        const missing = !!t.error;
        // A missing tool always gets "Download". An available tool only
        // gets an action button if vidpolish actually manages it (t.managed)
        // - there's nothing to redownload for a system-PATH ffmpeg/resvg.
        const showAction = missing || t.managed;
        const actionLabel = t.downloading
          ? t.total
            ? `Downloading... ${Math.min(100, Math.round((t.written / t.total) * 100))}%`
            : 'Downloading...'
          : missing
            ? 'Download'
            : 'Redownload';
        const pct = t.downloading && t.total ? Math.min(100, Math.round((t.written / t.total) * 100)) : null;
        return (
          <div key={t.name} class="card p-3 space-y-1 transition-colors hover:border-slate-700">
            <div class="flex items-center justify-between gap-3">
              <div class="flex items-center gap-2 min-w-0 shrink-0">
                {missing ? <XCircle size={16} class="text-red-400 shrink-0" /> : <CheckCircle2 size={16} class="text-emerald-400 shrink-0" />}
                <span class="font-medium whitespace-nowrap">{t.name}</span>
              </div>
              {showAction && (
                <button class="btn-secondary shrink-0" disabled={t.downloading} onClick={() => start(t.name, !missing)}>
                  {actionLabel}
                </button>
              )}
            </div>
            <div class="text-xs truncate" title={t.error || t.path}>
              {t.error ? (
                <span class="text-red-400">{t.error}</span>
              ) : (
                <span class="text-slate-400 font-mono">
                  {t.path}
                  {!t.managed && ' (system install, not managed by vidpolish)'}
                </span>
              )}
            </div>
            {t.downloading && (
              <div class="space-y-1 pt-1">
                <div class="h-1.5 w-full rounded-full bg-slate-800 overflow-hidden">
                  <div
                    class="h-full bg-sky-500 transition-[width] duration-200"
                    style={{ width: pct !== null ? `${pct}%` : '30%' }}
                  />
                </div>
                {t.total > 0 && (
                  <div class="text-xs text-slate-400 font-mono">
                    {formatBytes(t.written)} / {formatBytes(t.total)}
                  </div>
                )}
                {t.log && (
                  <div class="text-xs text-slate-500 font-mono truncate" title={t.log}>
                    {t.log}
                  </div>
                )}
              </div>
            )}
            {rowErrors[t.name] && <div class="text-xs text-red-400">{rowErrors[t.name]}</div>}
          </div>
        );
      })}
    </div>
  );
}
