import { useEffect, useState } from 'preact/hooks';

// Minimal hash-based router. No dependency: hash changes already push
// history entries (free back/forward support), and parsing a handful of
// path shapes doesn't need a library.
//
// Routes:
//   #/                                 -> { page: 'projects' }
//   #/projects/:id                     -> { page: 'project', projectId }
//   #/projects/:id/cells/:seq          -> { page: 'project', projectId, cellSeq }
//   #/config[/:section]                -> { page: 'config', section? } — section deep-links to one settings block
//   #/tools | #/cache                  -> { page: 'tools' | 'cache' }

function parseHash(hash) {
  const path = hash.replace(/^#/, '') || '/';
  const parts = path.split('/').filter(Boolean);

  if (parts[0] === 'projects' && parts[1]) {
    const route = { page: 'project', projectId: parts[1] };
    if (parts[2] === 'cells' && parts[3]) {
      route.cellSeq = Number(parts[3]);
    }
    return route;
  }
  if (parts[0] === 'config') {
    const route = { page: 'config' };
    if (parts[1]) route.section = parts[1];
    return route;
  }
  if (parts[0] === 'tools') return { page: 'tools' };
  if (parts[0] === 'cache') return { page: 'cache' };
  return { page: 'projects' };
}

export function useHashRoute() {
  const [route, setRoute] = useState(() => parseHash(location.hash));

  useEffect(() => {
    const onChange = () => setRoute(parseHash(location.hash));
    window.addEventListener('hashchange', onChange);
    return () => window.removeEventListener('hashchange', onChange);
  }, []);

  return route;
}

export function navigate(path) {
  location.hash = path;
}

// useDocumentTitle sets the browser tab title, appending the app name so
// every page still reads as "part of vidpolish" while telling tabs apart
// at a glance once several are open. Pass null/undefined to leave the tab
// title untouched — e.g. a page whose own title depends on data it's
// still fetching (ProjectView) passes nothing until that data is in,
// rather than clobbering whatever title another component already set.
export function useDocumentTitle(title) {
  useEffect(() => {
    if (!title) return;
    document.title = `${title} · vidpolish`;
  }, [title]);
}

export const paths = {
  home: () => '#/',
  project: (id) => `#/projects/${id}`,
  cell: (projectId, seq) => `#/projects/${projectId}/cells/${seq}`,
  config: (section) => (section ? `#/config/${section}` : '#/config'),
  tools: () => '#/tools',
  cache: () => '#/cache',
};
