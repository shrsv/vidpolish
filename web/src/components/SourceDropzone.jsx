import { useEffect, useRef, useState } from 'preact/hooks';
import { UploadCloud } from 'lucide-preact';
import { api } from '../api.js';

// Running inside the Wails GUI (vs. a plain browser via `vidpolish ui`)?
// window.go.main.App.PickVideoFile is only injected when the app binds
// that Go struct (see cmd/vidpolish-gui/app.go) - never true in a browser.
const isWailsGUI = typeof window !== 'undefined' && !!window.go?.main?.App?.PickVideoFile;

export function SourceDropzone({ cellId, onDone }) {
  const [dragging, setDragging] = useState(false);
  const [progress, setProgress] = useState(null);
  const [copying, setCopying] = useState(false);
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
    try {
      await api.uploadSource(cellId, file, setProgress);
      onDone();
    } catch (e) {
      setError(e.message);
    } finally {
      setProgress(null);
    }
  };

  const sendPath = async (path) => {
    setError('');
    setCopying(true);
    try {
      await api.uploadSourceFromPath(cellId, path);
      onDone();
    } catch (e) {
      setError(e.message);
    } finally {
      setCopying(false);
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
      ) : copying ? (
        <p class="text-sm text-cyan-400">Copying video...</p>
      ) : (
        <p class="text-sm text-cyan-400">Uploading... {Math.round(progress * 100)}%</p>
      )}
      {error && <p class="text-sm text-red-400 mt-2">{error}</p>}
    </div>
  );
}
