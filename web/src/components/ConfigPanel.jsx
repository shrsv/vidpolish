import { useEffect, useRef, useState } from 'preact/hooks';
import { Save, LogIn, CheckCircle2, History, RotateCcw } from 'lucide-preact';
import { api } from '../api.js';

export function ConfigPanel({ section }) {
  const [cfg, setCfg] = useState(null);
  const [clientSecretInput, setClientSecretInput] = useState('');
  const [saved, setSaved] = useState(false);
  const [saveError, setSaveError] = useState('');
  const [loginStatus, setLoginStatus] = useState('');
  const [backups, setBackups] = useState([]);
  const [showBackups, setShowBackups] = useState(false);
  const sectionRefs = useRef({});

  const refresh = () => api.getConfig().then(setCfg);
  const refreshBackups = () => api.listConfigBackups().then(setBackups);
  useEffect(refresh, []);
  useEffect(refreshBackups, []);

  // Deep-link support: #/config/thumbnail etc. scrolls to and briefly
  // highlights that settings section.
  useEffect(() => {
    if (!section || !cfg) return;
    const el = sectionRefs.current[section];
    if (!el) return;
    el.scrollIntoView({ behavior: 'smooth', block: 'start' });
    el.classList.add('ring-2', 'ring-cyan-500');
    const t = setTimeout(() => el.classList.remove('ring-2', 'ring-cyan-500'), 1600);
    return () => clearTimeout(t);
  }, [section, cfg]);

  if (!cfg) return <p class="text-sm text-slate-500">Loading...</p>;

  const set = (path, value) => {
    setCfg((c) => {
      const next = structuredClone(c);
      let obj = next;
      const parts = path.split('.');
      for (let i = 0; i < parts.length - 1; i++) obj = obj[parts[i]];
      obj[parts[parts.length - 1]] = value;
      return next;
    });
  };

  const save = async () => {
    setSaveError('');
    const body = {
      youtube: {
        clientId: cfg.youtube.clientId,
        privacy: cfg.youtube.privacy,
        defaultLanguage: cfg.youtube.defaultLanguage,
        defaultTags: cfg.youtube.defaultTags,
        descriptionTemplate: cfg.youtube.descriptionTemplate,
      },
      thumbnail: cfg.thumbnail,
    };
    if (clientSecretInput) body.youtube.clientSecret = clientSecretInput;
    try {
      await api.putConfig(body);
    } catch (e) {
      // Validation rejected the save; the on-disk config is untouched.
      setSaveError(e.message);
      return;
    }
    setClientSecretInput('');
    setSaved(true);
    setTimeout(() => setSaved(false), 2000);
    refresh();
    refreshBackups();
  };

  const restore = async (timestamp) => {
    if (!confirm('Restore config to this backup? Your current config will itself be backed up first.')) return;
    await api.restoreConfigBackup(timestamp);
    refresh();
    refreshBackups();
  };

  const connectYouTube = async () => {
    setLoginStatus('opening browser...');
    const { authUrl } = await api.startYouTubeLogin();
    window.open(authUrl, '_blank');
    const stop = api.subscribeYouTubeLoginEvents((msg) => {
      setLoginStatus(msg);
      if (msg === 'done' || msg.startsWith('error')) {
        stop();
        refresh();
      }
    });
  };

  return (
    <div class="space-y-6 max-w-2xl">
      <section class="card p-4 space-y-3" ref={(el) => (sectionRefs.current.youtube = el)}>
        <h2 class="font-semibold">YouTube API credentials</h2>
        <div>
          <label class="label">Client ID</label>
          <input class="input" value={cfg.youtube.clientId} onInput={(e) => set('youtube.clientId', e.currentTarget.value)} />
        </div>
        <div>
          <label class="label">
            Client secret {cfg.youtube.clientSecret.set && <span class="text-slate-500">(set: {cfg.youtube.clientSecret.preview})</span>}
          </label>
          <input
            class="input"
            type="password"
            placeholder={cfg.youtube.clientSecret.set ? 'leave blank to keep current value' : ''}
            value={clientSecretInput}
            onInput={(e) => setClientSecretInput(e.currentTarget.value)}
          />
        </div>
        <div class="flex items-center gap-2 text-sm">
          {cfg.youtube.refreshToken.set ? (
            <span class="flex items-center gap-1 text-emerald-400">
              <CheckCircle2 size={15} /> Connected to YouTube
            </span>
          ) : (
            <span class="text-slate-500">Not connected</span>
          )}
          <button class="btn-secondary ml-auto" onClick={connectYouTube}>
            <LogIn size={15} /> {cfg.youtube.refreshToken.set ? 'Reconnect' : 'Connect YouTube'}
          </button>
        </div>
        {loginStatus && <p class="text-xs text-cyan-400">{loginStatus}</p>}
      </section>

      <section class="card p-4 space-y-3" ref={(el) => (sectionRefs.current['upload-defaults'] = el)}>
        <h2 class="font-semibold">Upload defaults</h2>
        <div class="flex gap-4">
          <div class="flex-1">
            <label class="label">Privacy</label>
            <select class="input" value={cfg.youtube.privacy} onChange={(e) => set('youtube.privacy', e.currentTarget.value)}>
              <option value="unlisted">Unlisted</option>
              <option value="public">Public</option>
              <option value="private">Private</option>
            </select>
          </div>
          <div class="flex-1">
            <label class="label">Language</label>
            <input class="input" value={cfg.youtube.defaultLanguage} onInput={(e) => set('youtube.defaultLanguage', e.currentTarget.value)} />
          </div>
        </div>
        <div>
          <label class="label">Default tags (comma-separated)</label>
          <input
            class="input"
            value={cfg.youtube.defaultTags.join(', ')}
            onInput={(e) => set('youtube.defaultTags', e.currentTarget.value.split(',').map((t) => t.trim()).filter(Boolean))}
          />
        </div>
        <div>
          <label class="label">Description template</label>
          <textarea
            class="input font-mono"
            rows={3}
            value={cfg.youtube.descriptionTemplate}
            onInput={(e) => set('youtube.descriptionTemplate', e.currentTarget.value)}
          />
        </div>
      </section>

      <section class="card p-4 space-y-3" ref={(el) => (sectionRefs.current.thumbnail = el)}>
        <div class="flex items-center justify-between">
          <h2 class="font-semibold">Thumbnails</h2>
          <label class="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={cfg.thumbnail.enabled} onChange={(e) => set('thumbnail.enabled', e.currentTarget.checked)} />
            Auto-generate
          </label>
        </div>
        <div>
          <label class="label">Logo path (PNG/JPG/SVG, local file)</label>
          <input class="input" value={cfg.thumbnail.logoPath} onInput={(e) => set('thumbnail.logoPath', e.currentTarget.value)} />
        </div>
        <div class="flex gap-4">
          {['backgroundColor', 'accentColor', 'textColor'].map((field) => (
            <div key={field} class="flex-1">
              <label class="label">{field.replace('Color', '')}</label>
              <div class="flex items-center gap-2">
                <input
                  class="h-9 w-9 shrink-0 rounded-md border border-slate-700 bg-transparent p-0.5"
                  type="color"
                  value={cfg.thumbnail[field]}
                  onInput={(e) => set(`thumbnail.${field}`, e.currentTarget.value)}
                />
                <input
                  class="input font-mono"
                  value={cfg.thumbnail[field]}
                  onInput={(e) => set(`thumbnail.${field}`, e.currentTarget.value)}
                />
              </div>
            </div>
          ))}
        </div>
      </section>

      {saveError && (
        <p class="text-sm text-red-400">
          Not saved: {saveError} — your existing config is untouched.
        </p>
      )}
      <div class="flex items-center gap-3">
        <button class="btn-primary" onClick={save}>
          <Save size={15} /> {saved ? 'Saved' : 'Save config'}
        </button>
        <button class="btn-secondary" onClick={() => setShowBackups((v) => !v)}>
          <History size={15} /> Backups ({backups.length})
        </button>
      </div>

      {showBackups && (
        <section class="card p-4 space-y-2">
          <p class="text-xs text-slate-500">
            A snapshot of config.toml is taken automatically before every save. Restoring itself
            creates a new backup first, so nothing here is ever a one-way trip.
          </p>
          {backups.length === 0 && <p class="text-sm text-slate-500">No backups yet.</p>}
          {backups.map((b) => (
            <div key={b.timestamp} class="flex items-center justify-between text-sm">
              <span class="text-slate-400 font-mono">
                {new Date(Number(BigInt(b.timestamp) / 1000000n)).toLocaleString()}
              </span>
              <button class="btn-secondary" onClick={() => restore(b.timestamp)}>
                <RotateCcw size={13} /> Restore
              </button>
            </div>
          ))}
        </section>
      )}
    </div>
  );
}
