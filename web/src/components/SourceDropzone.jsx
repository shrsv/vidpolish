import { useEffect, useRef, useState } from 'preact/hooks';
import { UploadCloud, Loader2 } from 'lucide-preact';
import { api } from '../api.js';
import { ProgressStepper, tryParseProgress } from './ProgressStepper.jsx';

// Running inside the Wails GUI (vs. a plain browser via `vidpolish ui`)?
// window.go.main.App.PickVideoFile is only injected when the app binds
// that Go struct (see cmd/vidpolish-gui/app.go) - never true in a browser.
const isWailsGUI = typeof window !== 'undefined' && !!window.go?.main?.App?.PickVideoFile;

export function SourceDropzone({ cellId, onDone }) {
  const [dragging, setDragging] = useState(false);
  const [progress, setProgress] = useState(null); // network transfer fraction, browser upload path only
  const [copying, setCopying] = useState(false);
  const [live, setLive] = useState(''); // raw per-cell status message, polled while busy
  const [error, setError] = useState('');
  const inputRef = useRef();

  // In the Wails GUI, a file dropped or picked from a browser-side File
  // object can come back with unreadable/empty content (WebView2
  // limitation - see app.go's doc comment), so drops there are handled
  // exclusively through window.runtime.OnFileDrop below, which hands back
  // a real OS path instead. This effect only matters under Wails.
  useEffect(() => {
    if (!isWailsGUI || !window.runtime?.OnFileDrop) return;
    window.runtime.OnFileDrop((_x, _y, paths) => {
      if (paths && paths[0]) sendPath(paths[0]);
    }, true);
    return () => window.runtime.OnFileDropOff?.();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cellId]);

  const send = async (file) => {
    if (!file) return;
    setError('');
    setProgress(0);
    setLive('');
    let poll = null;
    try {
      await api.uploadSource(cellId, file, (fraction) => {
        setProgress(fraction);
        // Once the network transfer itself finishes, the backend keeps
        // going with faststart/poster/preview steps before the request
        // resolves (see writeSourceVideo) - start polling the same
        // per-cell status the Wails path already does, instead of
        // sitting at "100%" for however long those steps take, which
        // otherwise looks exactly like the app is stuck.
        if (fraction >= 1 && !poll) {
          poll = setInterval(() => {
            api
              .getCell(cellId)
              .then((c) => setLive(c.statusMessage || ''))
              .catch(() => {});
          }, 400);
        }
      });
      onDone();
    } catch (e) {
      setError(e.message);
    } finally {
      if (poll) clearInterval(poll);
      setProgress(null);
      setLive('');
    }
  };

  const sendPath = async (path) => {
    setError('');
    setCopying(true);
    setLive('');
    // The upload itself is a single blocking request (see
    // api.uploadSourceFromPath) with no progress of its own - the backend
    // publishes real multi-step status on the same per-cell status/SSE
    // channel an edit cell's own run progress uses (copy -> faststart
    // remux -> poster -> preview proxy re-encode, the full step list
    // known upfront - see writeSourceVideo), so poll that in parallel
    // instead of showing a static "Copying video..." for the whole
    // multi-stage process, which otherwise looks identical to a frozen
    // app.
    const poll = setInterval(() => {
      api
        .getCell(cellId)
        .then((c) => setLive(c.statusMessage || ''))
        .catch(() => {});
    }, 400);
    try {
      await api.uploadSourceFromPath(cellId, path);
      onDone();
    } catch (e) {
      setError(e.message);
    } finally {
      clearInterval(poll);
      setCopying(false);
      setLive('');
    }
  };

  const choose = async () => {
    if (isWailsGUI) {
      try {
        const path = await window.go.main.App.PickVideoFile();
        if (path) sendPath(path);
      } catch (e) {
        setError(e.message || String(e));
      }
      return;
    }
    inputRef.current.click();
  };

  const busy = progress !== null || copying;
  const progressData = tryParseProgress(live);

  return (
    <div
      class={`card border-dashed border-2 p-10 text-center transition-colors ${
        dragging ? 'border-cyan-500 bg-cyan-950/20' : 'border-slate-700'
      }`}
      style={isWailsGUI ? { '--wails-drop-target': 'drop' } : undefined}
      onDragOver={(e) => {
        e.preventDefault();
        setDragging(true);
      }}
      onDragLeave={() => setDragging(false)}
      onDrop={(e) => {
        e.preventDefault();
        setDragging(false);
        // Under Wails, the real upload happens via the OnFileDrop effect
        // above (real OS path); e.dataTransfer.files here is the
        // unreliable browser File object, so it's deliberately ignored.
        if (!isWailsGUI) send(e.dataTransfer.files[0]);
      }}
    >
      <UploadCloud class="mx-auto mb-2 text-slate-500" size={32} />
      {!busy ? (
        <>
          <p class="text-sm text-slate-300">Drag a video file here, or</p>
          <button class="btn-secondary mt-2" onClick={choose}>
            Choose file
          </button>
          {!isWailsGUI && (
            <input
              ref={inputRef}
              type="file"
              accept="video/*"
              class="hidden"
              onChange={(e) => send(e.currentTarget.files[0])}
            />
          )}
        </>
      ) : progressData ? (
        <div class="max-w-xs mx-auto text-left">
          <ProgressStepper progress={progressData} />
        </div>
      ) : (
        <UploadProgressBar
          label={live || (copying ? 'Copying video...' : 'Uploading...')}
          percent={progress != null ? Math.round(progress * 100) : null}
        />
      )}
      {error && <p class="text-sm text-red-400 mt-2">{error}</p>}
    </div>
  );
}

// UploadProgressBar is the plain fallback (no full step list known yet -
// e.g. the browser upload path's raw network-transfer percentage, or a
// plain-text status line) for whenever tryParseProgress can't give us a
// full ProgressStepper.
function UploadProgressBar({ label, percent }) {
  return (
    <div class="space-y-1 w-full max-w-xs mx-auto">
      <div class="flex items-center justify-between text-xs text-cyan-400">
        <span class="flex items-center gap-1.5">
          <Loader2 size={12} class="animate-spin" /> {label}
        </span>
      </div>
      <div class="h-1.5 w-full rounded-full bg-slate-800 overflow-hidden">
        <div
          class={`h-full bg-cyan-500 ${percent == null ? 'animate-pulse w-1/3' : 'transition-all'}`}
          style={percent != null ? { width: `${percent}%` } : undefined}
        />
      </div>
      {percent != null && <p class="text-xs text-cyan-400">{percent}%</p>}
    </div>
  );
}
