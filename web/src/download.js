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

// buildDownloadName prefixes a download filename with the project's
// original source filename (e.g. the raw screen recording someone dropped
// in), so a downloaded edit/GIF stays traceable back to what it came
// from instead of only carrying the cell's own generic label ("Edit #2").
// Falls back to the cell name alone if the project has no source cell or
// it has no recorded source filename yet (still mid-upload).
export function buildDownloadName(project, cellName, ext) {
  const source = project?.cells?.find((c) => c.kind === 'source');
  const sourceFilename = source?.sourceFilename;
  if (!sourceFilename) {
    return `${sanitizeFilename(cellName)}${ext}`;
  }
  const sourceStem = sourceFilename.replace(/\.[^./\\]+$/, '');
  return `${sanitizeFilename(sourceStem)}-${sanitizeFilename(cellName)}${ext}`;
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
