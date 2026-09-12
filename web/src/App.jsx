import { useState } from 'preact/hooks';
import { Film, Settings, Wrench, Database } from 'lucide-preact';
import { ProjectList } from './components/ProjectList.jsx';
import { ProjectView } from './components/ProjectView.jsx';
import { ConfigPanel } from './components/ConfigPanel.jsx';
import { ToolsPanel } from './components/ToolsPanel.jsx';
import { CachePanel } from './components/CachePanel.jsx';

const TABS = [
  { id: 'projects', label: 'Projects', icon: Film },
  { id: 'config', label: 'Config', icon: Settings },
  { id: 'tools', label: 'Tools', icon: Wrench },
  { id: 'cache', label: 'Cache', icon: Database },
];

export function App() {
  const [tab, setTab] = useState('projects');
  const [openProjectId, setOpenProjectId] = useState(null);

  return (
    <div class="min-h-screen flex flex-col">
      <header class="border-b border-slate-800 bg-slate-900/60 backdrop-blur px-4 py-3 flex items-center gap-6 sticky top-0 z-10">
        <div class="flex items-center gap-2 font-semibold text-cyan-400">
          <Film size={20} />
          vidpolish
        </div>
        <nav class="flex gap-1">
          {TABS.map(({ id, label, icon: Icon }) => (
            <button
              key={id}
              class={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium ${
                tab === id ? 'bg-slate-800 text-white' : 'text-slate-400 hover:text-slate-200'
              }`}
              onClick={() => {
                setTab(id);
                if (id !== 'projects') setOpenProjectId(null);
              }}
            >
              <Icon size={15} />
              {label}
            </button>
          ))}
        </nav>
      </header>

      <main class="flex-1 p-6 max-w-5xl mx-auto w-full">
        {tab === 'projects' && !openProjectId && (
          <ProjectList onOpen={setOpenProjectId} />
        )}
        {tab === 'projects' && openProjectId && (
          <ProjectView projectId={openProjectId} onBack={() => setOpenProjectId(null)} />
        )}
        {tab === 'config' && <ConfigPanel />}
        {tab === 'tools' && <ToolsPanel />}
        {tab === 'cache' && <CachePanel />}
      </main>
    </div>
  );
}
