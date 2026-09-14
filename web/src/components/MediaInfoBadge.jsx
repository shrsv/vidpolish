import { useEffect, useState } from 'preact/hooks';
import { Info } from 'lucide-preact';
import { api } from '../api.js';

// formatDuration turns seconds into "3:12" or "1:03:12".
export function formatDuration(sec) {
  if (sec == null) return '—';
  sec = Math.round(sec);
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
  return `${m}:${String(s).padStart(2, '0')}`;
}

// formatBitrate turns kbps into "8.1 Mbps" or "512 kbps".
export function formatBitrate(kbps) {
  if (!kbps) return '—';
  return kbps >= 1000 ? `${(kbps / 1000).toFixed(1)} Mbps` : `${Math.round(kbps)} kbps`;
}

// formatSize turns bytes into "245 MB" / "1.2 GB".
export function formatSize(bytes) {
  if (bytes == null) return '—';
  if (bytes >= 1e9) return `${(bytes / 1e9).toFixed(2)} GB`;
  if (bytes >= 1e6) return `${(bytes / 1e6).toFixed(1)} MB`;
  if (bytes >= 1e3) return `${(bytes / 1e3).toFixed(0)} KB`;
  return `${bytes} B`;
}

// formatPct renders a signed percent, e.g. "+12%" / "-60%".
export function formatPct(pct) {
  if (pct == null || Number.isNaN(pct)) return '—';
  const sign = pct > 0 ? '+' : '';
  return `${sign}${pct.toFixed(0)}%`;
}

// pctClass colors a change: shrinking (negative) reads as a mild positive
// (emerald), growing (positive) as a mild caution (amber) — file size and
// bitrate usually shrink on purpose here, so this is a hint, not a verdict.
function pctClass(pct) {
  if (pct == null || Math.abs(pct) < 1) return 'text-slate-400';
  return pct < 0 ? 'text-emerald-400' : 'text-amber-400';
}

function Row({ label, value, valueClass = 'text-slate-200' }) {
  return (
    <div class="flex items-center justify-between gap-3">
      <span class="text-slate-500">{label}</span>
      <span class={`font-mono ${valueClass}`}>{value}</span>
    </div>
  );
}

// MediaInfoBadge shows a compact, always-visible summary of a cell's video
// (resolution · duration · bitrate · size) bottom-right, and a full
// details popover — including a comparison against the source, for edit
// cells — on hover/click. Renders nothing until the cell actually has a
// video to describe.
export function MediaInfoBadge({ cellId, status }) {
  const [info, setInfo] = useState(null);
  const [open, setOpen] = useState(false);

  useEffect(() => {
    api.getCellInfo(cellId).then(setInfo).catch(() => setInfo(null));
  }, [cellId, status]);

  if (!info?.self) return null;
  const { self, comparedToSource } = info;

  const summary = `${self.width}×${self.height} · ${formatDuration(self.durationSec)} · ${formatBitrate(self.bitrateKbps)} · ${formatSize(self.sizeBytes)}`;

  return (
    <div class="relative flex justify-end">
      <button
        type="button"
        class="inline-flex items-center gap-1 text-[11px] text-slate-500 hover:text-cyan-400 font-mono transition-colors"
        onClick={() => setOpen((o) => !o)}
        onMouseEnter={() => setOpen(true)}
        onMouseLeave={() => setOpen(false)}
      >
        <Info size={11} /> {summary}
      </button>
      {open && (
        <div class="absolute bottom-full right-0 mb-1.5 w-64 rounded-md border border-slate-700 bg-slate-950 p-3 text-xs shadow-xl z-10 space-y-1">
          <Row label="Resolution" value={`${self.width}×${self.height}`} />
          <Row label="Duration" value={formatDuration(self.durationSec)} />
          <Row label="Video bitrate" value={formatBitrate(self.bitrateKbps)} />
          {self.audioBitrateKbps > 0 && <Row label="Audio bitrate" value={formatBitrate(self.audioBitrateKbps)} />}
          <Row label="Frame rate" value={self.frameRate ? `${self.frameRate.toFixed(2)} fps` : '—'} />
          <Row label="File size" value={formatSize(self.sizeBytes)} />
          {comparedToSource && (
            <>
              <div class="pt-1.5 mt-1.5 border-t border-slate-800 text-slate-500">vs. source</div>
              <Row label="Size" value={formatPct(comparedToSource.sizeChangePct)} valueClass={pctClass(comparedToSource.sizeChangePct)} />
              <Row label="Duration" value={formatPct(comparedToSource.durationChangePct)} valueClass={pctClass(comparedToSource.durationChangePct)} />
              <Row label="Bitrate" value={formatPct(comparedToSource.bitrateChangePct)} valueClass={pctClass(comparedToSource.bitrateChangePct)} />
              {Math.abs(comparedToSource.resolutionChangePct) >= 1 && (
                <Row
                  label="Resolution (area)"
                  value={formatPct(comparedToSource.resolutionChangePct)}
                  valueClass={pctClass(comparedToSource.resolutionChangePct)}
                />
              )}
            </>
          )}
        </div>
      )}
    </div>
  );
}
