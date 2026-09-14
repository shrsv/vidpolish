// sanitizeFilename turns a cell's display name (which may contain
// "#", ":", "/", or other characters that are awkward or outright
// invalid in filenames on some OSes/filesystems) into a safe download
// filename stem — spaces become hyphens, anything else that isn't
// alphanumeric/./_/- is dropped.
export function sanitizeFilename(name) {
  return name
    .trim()
    .replace(/\s+/g, '-')
    .replace(/[^A-Za-z0-9._-]/g, '') || 'download';
}

// saveBlob saves an in-memory blob to disk, preferring a real OS
// save-file dialog (Chromium's File System Access API) so the user picks
// where it goes, falling back to a plain object-URL download link.
export async function saveBlob(blob, suggestedName, { description, accept } = {}) {
  if (window.showSaveFilePicker) {
    try {
      const handle = await window.showSaveFilePicker({
        suggestedName,
        types: accept ? [{ description, accept }] : undefined,
      });
      const writable = await handle.createWritable();
      await writable.write(blob);
      await writable.close();
      return;
    } catch (e) {
      if (e.name === 'AbortError') return; // user cancelled the picker
      // fall through to the plain download link on any other failure
    }
  }
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = suggestedName;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 10000);
}
