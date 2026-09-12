import { Film, Settings, Wrench, Database } from 'lucide-preact';
import { ProjectList } from './components/ProjectList.jsx';
import { ProjectView } from './components/ProjectView.jsx';
import { ConfigPanel } from './components/ConfigPanel.jsx';
import { ToolsPanel } from './components/ToolsPanel.jsx';
import { CachePanel } from './components/CachePanel.jsx';
import { useHashRoute, navigate, paths } from './router.js';

const TABS = [
  { id: 'projects', label: 'Projects', icon: Film, path: paths.home() },
  { id: 'config', label: 'Config', icon: Settings, path: paths.config() },
  { id: 'tools', label: 'Tools', icon: Wrench, path: paths.tools() },
  { id: 'cache', label: 'Cache', icon: Database, path: paths.cache() },
];

export function App() {
  const route = useHashRoute();
  const activePage = route.page === 'project' ? 'projects' : route.page;

  return (
    <div class="min-h-screen flex flex-col">
      <header class="border-b border-slate-800 bg-slate-900/60 backdrop-blur sticky top-0 z-10">
        <div class="max-w-5xl mx-auto px-6 py-3 flex items-center gap-6">
          <button
            class="flex items-center gap-2 font-semibold text-cyan-400 hover:text-cyan-300 transition-colors"
            onClick={() => navigate(paths.home())}
            title="Go to projects"
          >
            <img src="/logo.svg" alt="" class="h-6 w-6 rounded" />
            vidpolish
          </button>
          <nav class="flex gap-1">
            {TABS.map(({ id, label, icon: Icon, path }) => (
              <button
                key={id}
                class={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
                  activePage === id ? 'bg-slate-800 text-white' : 'text-slate-400 hover:text-slate-200'
                }`}
                onClick={() => navigate(path)}
              >
                <Icon size={15} />
                {label}
              </button>
            ))}
          </nav>
        </div>
      </header>

      <main class="flex-1 p-6 max-w-5xl mx-auto w-full">
        {route.page === 'projects' && <ProjectList />}
        {route.page === 'project' && (
          <ProjectView projectId={route.projectId} initialCellSeq={route.cellSeq} />
        )}
        {route.page === 'config' && <ConfigPanel section={route.section} />}
        {route.page === 'tools' && <ToolsPanel />}
        {route.page === 'cache' && <CachePanel />}
      </main>
    </div>
  );
}
