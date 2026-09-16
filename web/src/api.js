async function request(method, path, body) {
  const res = await fetch(path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  if (res.status === 204) return null;
  const data = await res.json().catch(() => null);
  if (!res.ok) throw new Error(data?.error || res.statusText);
  return data;
}

export const api = {
  listProjects: () => request('GET', '/api/projects'),
  createProject: (name) => request('POST', '/api/projects', { name }),
  getProject: (id) => request('GET', `/api/projects/${id}`),
  deleteProject: (id) => request('DELETE', `/api/projects/${id}`),

  createCell: (projectId, body) => request('POST', `/api/projects/${projectId}/cells`, body),
  updateCell: (id, body) => request('PATCH', `/api/cells/${id}`, body),
  getCell: (id) => request('GET', `/api/cells/${id}`),
  getCellInfo: (id) => request('GET', `/api/cells/${id}/info`),
  runCell: (id) => request('POST', `/api/cells/${id}/run`),
  deleteCell: (id) => request('DELETE', `/api/cells/${id}`),
  reorderCells: (projectId, kind, orderedCellIds) =>
    request('POST', `/api/projects/${projectId}/reorder`, { kind, orderedCellIds }),

  uploadSource: async (cellId, file, onProgress) => {
    return new Promise((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      xhr.open('POST', `/api/cells/${cellId}/source`);
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable && onProgress) onProgress(e.loaded / e.total);
      };
      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          resolve(JSON.parse(xhr.responseText));
        } else {
          reject(new Error(xhr.responseText || xhr.statusText));
        }
      };
      xhr.onerror = () => reject(new Error('upload failed'));
      const form = new FormData();
      form.append('video', file);
      xhr.send(form);
    });
  },

  // uploadSourceFromPath is uploadSource's counterpart for the Wails GUI
  // (see SourceDropzone.jsx): reads the video directly off disk by
  // absolute path via the Go backend, instead of a browser File object
  // read over the request body. Only meant to be called with a path that
  // came from window.go.main.App.PickVideoFile or window.runtime.OnFileDrop
  // - a WebView2-hosted page's own File objects can come back unreadable
  // (see internal/server/handlers_run.go's writeSourceVideo doc comment).
  uploadSourceFromPath: (cellId, path) => request('POST', `/api/cells/${cellId}/source-path`, { path }),

  subscribeCellEvents: (cellId, onMessage) => {
    const es = new EventSource(`/api/cells/${cellId}/events`);
    es.onmessage = (e) => {
      try {
        onMessage(JSON.parse(e.data));
      } catch {
        onMessage(e.data);
      }
    };
    return () => es.close();
  },

  previewThumbnail: async (title) => {
    const res = await fetch('/api/thumbnail/preview', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title }),
    });
    if (!res.ok) {
      const data = await res.json().catch(() => null);
      throw new Error(data?.error || res.statusText);
    }
    return res.blob();
  },

  exportGif: async (cellId, { fps, width } = {}) => {
    const res = await fetch(`/api/cells/${cellId}/export-gif`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ fps, width }),
    });
    if (!res.ok) {
      const data = await res.json().catch(() => null);
      throw new Error(data?.error || res.statusText);
    }
    return res.blob();
  },

  getConfig: () => request('GET', '/api/config'),
  putConfig: (body) => request('PUT', '/api/config', body),
  listConfigBackups: () => request('GET', '/api/config/backups'),
  restoreConfigBackup: (timestamp) => request('POST', `/api/config/backups/${timestamp}/restore`, {}),
  startYouTubeLogin: () => request('POST', '/api/youtube/login', {}),
  subscribeYouTubeLoginEvents: (onMessage) => {
    const es = new EventSource('/api/youtube/login/events');
    es.onmessage = (e) => onMessage(e.data);
    return () => es.close();
  },

  // listTools also reports live progress for anything downloading right
  // now (see the Downloading/Written/Total/Log fields) - poll it while any
  // row is downloading instead of a separate events subscription, so
  // progress survives navigating away and back (the state lives on the
  // server, not in this tab's component tree).
  listTools: () => request('GET', '/api/tools'),
  resolveTool: (name) => request('POST', `/api/tools/${name}/resolve`),
  redownloadTool: (name) => request('POST', `/api/tools/${name}/redownload`),
  resolveAllTools: () => request('POST', '/api/tools/resolve-all'),

  listCache: () => request('GET', '/api/cache'),
  deleteCacheEntry: (fingerprint) => request('DELETE', `/api/cache/${fingerprint}`),
  cleanCache: () => request('POST', '/api/cache/clean'),
};
