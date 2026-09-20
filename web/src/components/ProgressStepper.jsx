import { Check, Loader2 } from 'lucide-preact';

// tryParseProgress attempts to parse a raw per-cell status message as the
// backend's structured multi-step progress JSON (see internal/server's
// `progress` struct, published by stepProgressReporter for any multi-stage
// cell task: a source upload, an edit cell's run, a YouTube upload cell's
// run). Returns null for anything else - a plain-text status line like
// "queued...", "starting...", "error: ...", or a raw log line - so
// callers can fall back to just showing that text, the way status
// messages have always been shown.
export function tryParseProgress(raw) {
  if (!raw) return null;
  try {
    const p = JSON.parse(raw);
    if (p && Array.isArray(p.steps) && p.steps.length > 0) return p;
  } catch {
    // Not JSON - a plain-text status line.
  }
  return null;
}

// ProgressStepper renders every step of a multi-stage cell task at once -
// ticked for the ones already done, a live percentage/ETA for whichever
// is active, greyed out for what hasn't started - instead of a single
// line of text that just changes in place with no sense of how much is
// left or how many stages there even are.
export function ProgressStepper({ progress }) {
  const { steps, stepIndex, stepPct, overallPct, eta } = progress;
  const activeLabel = steps[stepIndex] ?? '';

  return (
    <div class="space-y-2">
      <div class="flex items-center justify-between text-xs text-cyan-400">
        <span class="flex items-center gap-1.5">
          <Loader2 size={12} class="animate-spin" />
          {activeLabel}
          {stepPct != null && ` ${Math.round(stepPct)}%`}
        </span>
        <span class="text-slate-500">
          {Math.round(overallPct)}% overall{eta ? ` · ETA ${eta}` : ''}
        </span>
      </div>
      <div class="h-1.5 w-full rounded-full bg-slate-800 overflow-hidden">
        <div class="h-full bg-cyan-500 transition-all" style={{ width: `${overallPct}%` }} />
      </div>
      <div class="flex flex-wrap items-center gap-1">
        {steps.map((label, i) => {
          const state = i < stepIndex ? 'done' : i === stepIndex ? 'active' : 'pending';
          return (
            <span
              key={label}
              class={`text-[10px] px-1.5 py-0.5 rounded-full flex items-center gap-1 ${
                state === 'done'
                  ? 'bg-emerald-500/20 text-emerald-400'
                  : state === 'active'
                    ? 'bg-cyan-500/20 text-cyan-400'
                    : 'bg-slate-800 text-slate-500'
              }`}
            >
              {state === 'done' && <Check size={10} />}
              {state === 'active' && <Loader2 size={10} class="animate-spin" />}
              {label}
            </span>
          );
        })}
      </div>
    </div>
  );
}
