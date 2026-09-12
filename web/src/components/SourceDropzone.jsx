import { useRef, useState } from 'preact/hooks';
import { UploadCloud } from 'lucide-preact';
import { api } from '../api.js';

export function SourceDropzone({ cellId, onDone }) {
  const [dragging, setDragging] = useState(false);
  const [progress, setProgress] = useState(null);
  const [error, setError] = useState('');
  const inputRef = useRef();

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

  return (
    <div
      class={`card border-dashed border-2 p-10 text-center transition-colors ${
        dragging ? 'border-cyan-500 bg-cyan-950/20' : 'border-slate-700'
      }`}
      onDragOver={(e) => {
        e.preventDefault();
        setDragging(true);
      }}
      onDragLeave={() => setDragging(false)}
      onDrop={(e) => {
        e.preventDefault();
        setDragging(false);
        send(e.dataTransfer.files[0]);
      }}
    >
      <UploadCloud class="mx-auto mb-2 text-slate-500" size={32} />
      {progress === null ? (
        <>
          <p class="text-sm text-slate-300">Drag a video file here, or</p>
          <button class="btn-secondary mt-2" onClick={() => inputRef.current.click()}>
            Choose file
          </button>
          <input
            ref={inputRef}
            type="file"
            accept="video/*"
            class="hidden"
            onChange={(e) => send(e.currentTarget.files[0])}
          />
        </>
      ) : (
        <p class="text-sm text-cyan-400">Uploading... {Math.round(progress * 100)}%</p>
      )}
      {error && <p class="text-sm text-red-400 mt-2">{error}</p>}
    </div>
  );
}
