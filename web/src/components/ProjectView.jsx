import { useEffect, useRef, useState } from 'preact/hooks';
import { Plus, ChevronsDownUp, ChevronsUpDown, GripVertical } from 'lucide-preact';
import { api } from '../api.js';
import { SourceDropzone } from './SourceDropzone.jsx';
import { Cell } from './Cell.jsx';
import { MediaInfoBadge } from './MediaInfoBadge.jsx';
import { useDocumentTitle } from '../router.js';
import { timeAgo, fullTimestamp } from '../time.js';

export function ProjectView({ projectId, initialCellSeq }) {
  const [project, setProject] = useState(null);
  const [error, setError] = useState('');
  const [collapsed, setCollapsed] = useState(new Set());
  const [dragState, setDragState] = useState(null); // { kind, draggedId }

  const cellRefs = useRef({}); // seq -> element
  const sectionRefs = useRef({}); // 'source' | 'edit' | 'upload' -> element

  const refresh = () => api.getProject(projectId).then(setProject).catch((e) => setError(e.message));
  useEffect(refresh, [projectId]);

  // Deep-link: scroll to and briefly highlight the target cell whenever
  // the route's cellSeq changes (including clicking a "based on" link
  // while already on this project).
  useEffect(() => {
    if (!initialCellSeq || !project) return;
    const el = cellRefs.current[initialCellSeq];
    if (!el) return;
    el.scrollIntoView({ behavior: 'smooth', block: 'center' });
    el.classList.add('ring-2', 'ring-cyan-500');
    const t = setTimeout(() => el.classList.remove('ring-2', 'ring-cyan-500'), 1600);
    return () => clearTimeout(t);
  }, [initialCellSeq, project]);

  const deepLinkedCell = project && initialCellSeq ? project.cells.find((c) => c.seq === initialCellSeq) : null;
  useDocumentTitle(project && (deepLinkedCell ? `${deepLinkedCell.name} · ${project.name}` : project.name));

  if (error) return <p class="text-sm text-red-400">{error}</p>;
  if (!project) return <p class="text-sm text-slate-500">Loading...</p>;

  const source = project.cells.find((c) => c.kind === 'source');
  const editCells = project.cells.filter((c) => c.kind === 'edit');
  const uploadCells = project.cells.filter((c) => c.kind === 'upload');
  const textCells = project.cells.filter((c) => c.kind === 'text');
  const allCollapsibleIds = [...editCells, ...uploadCells, ...textCells].map((c) => c.id);
  const allCollapsed = allCollapsibleIds.length > 0 && allCollapsibleIds.every((id) => collapsed.has(id));

  const toggleCollapsed = (id) => {
    setCollapsed((prev) => {
      const next = new Set(prev);
      next.has(id) ? next.delete(id) : next.add(id);
      return next;
    });
  };
  const collapseAll = () => setCollapsed(allCollapsed ? new Set() : new Set(allCollapsibleIds));

  const scrollToSection = (key) => sectionRefs.current[key]?.scrollIntoView({ behavior: 'smooth', block: 'start' });

  const addEdit = async () => {
    await api.createCell(projectId, {
      kind: 'edit',
      parentCellId: source.id,
      params: { margin: '0.2s', speed: 1.0 },
    });
    refresh();
  };

  const addText = async () => {
    await api.createCell(projectId, {
      kind: 'text',
      parentCellId: source.id,
      params: { markdown: '' },
    });
    refresh();
  };

  const addUpload = async (editCellId) => {
    await api.createCell(projectId, {
      kind: 'upload',
      parentCellId: editCellId,
      params: { title: '', privacy: 'unlisted', tags: [] },
    });
    refresh();
  };

  // Drag-and-drop reordering, scoped to cells of one kind.
  const onDrop = async (kind, list, targetId) => {
    const draggedId = dragState?.draggedId;
    setDragState(null);
    if (!draggedId || draggedId === targetId) return;
    const ids = list.map((c) => c.id);
    const from = ids.indexOf(draggedId);
    const to = ids.indexOf(targetId);
    if (from === -1 || to === -1) return;
    ids.splice(to, 0, ids.splice(from, 1)[0]);
    // Optimistic local reorder.
    setProject((p) => ({
      ...p,
      cells: reorderLocal(p.cells, kind, ids),
    }));
    try {
      await api.reorderCells(projectId, kind, ids);
    } finally {
      refresh();
    }
  };

  return (
    <div class="space-y-6">
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-lg font-semibold">{project.name}</h1>
          <p
            class="text-xs text-slate-500"
            title={`Created ${fullTimestamp(project.createdAt)}\nUpdated ${fullTimestamp(project.updatedAt)}`}
          >
            Created {timeAgo(project.createdAt)} · updated {timeAgo(project.updatedAt)}
          </p>
        </div>
        <button class="btn-secondary" onClick={collapseAll}>
          {allCollapsed ? <ChevronsUpDown size={15} /> : <ChevronsDownUp size={15} />}
          {allCollapsed ? 'Expand all' : 'Collapse all'}
        </button>
      </div>

      {/* Category quick-nav */}
      <div class="sticky top-[57px] z-[5] -mx-6 px-6 py-2 bg-slate-950/90 backdrop-blur border-b border-slate-800 flex gap-4 text-sm">
        <button class="text-slate-400 hover:text-cyan-400 transition-colors" onClick={() => scrollToSection('source')}>
          Source
        </button>
        <button class="text-slate-400 hover:text-cyan-400 transition-colors" onClick={() => scrollToSection('edit')}>
          Edit cells ({editCells.length})
        </button>
        <button class="text-slate-400 hover:text-cyan-400 transition-colors" onClick={() => scrollToSection('upload')}>
          Upload cells ({uploadCells.length})
        </button>
        <button class="text-slate-400 hover:text-cyan-400 transition-colors" onClick={() => scrollToSection('text')}>
          Notes ({textCells.length})
        </button>
      </div>

      {/* Source cell */}
      <div ref={(el) => (sectionRefs.current.source = el)}>
        {source.mediaUrl ? (
          <div class="card p-4 space-y-2" ref={(el) => (cellRefs.current[source.seq] = el)}>
            <div class="flex items-center gap-2">
              <span class="text-xs font-mono text-slate-500">#{source.seq}</span>
              <span class="font-medium">{source.name}</span>
              <span class="text-xs text-slate-500">{source.sourceFilename}</span>
              <span
                class="text-xs text-slate-600"
                title={`Created ${fullTimestamp(source.createdAt)}\nUpdated ${fullTimestamp(source.updatedAt)}`}
              >
                updated {timeAgo(source.updatedAt)}
              </span>
            </div>
            <video controls src={source.mediaUrl} class="w-full rounded-md max-h-80" />
            <MediaInfoBadge cellId={source.id} status={source.status} />
          </div>
        ) : (
          <SourceDropzone cellId={source.id} onDone={refresh} />
        )}
      </div>

      {source.mediaUrl && (
        <>
          <div ref={(el) => (sectionRefs.current.edit = el)} class="space-y-4">
            <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wide">Edit cells</h2>
            {editCells.map((c) => (
              <DraggableCell
                key={c.id}
                cellRef={(el) => (cellRefs.current[c.seq] = el)}
                dragging={dragState?.kind === 'edit' && dragState.draggedId === c.id}
                onDragStart={() => setDragState({ kind: 'edit', draggedId: c.id })}
                onDragOver={(e) => e.preventDefault()}
                onDrop={() => onDrop('edit', editCells, c.id)}
              >
                <Cell
                  cell={c}
                  editCells={editCells}
                  project={project}
                  collapsed={collapsed.has(c.id)}
                  onToggleCollapse={() => toggleCollapsed(c.id)}
                  onChanged={refresh}
                  onDelete={refresh}
                  onAddUpload={c.mediaUrl ? () => addUpload(c.id) : undefined}
                />
              </DraggableCell>
            ))}
            {editCells.length === 0 && (
              <p class="text-sm text-slate-500">No edit cells yet. Add one to denoise/cut at a chosen margin and speed.</p>
            )}
            <button class="btn-secondary" onClick={addEdit}>
              <Plus size={15} /> Add edit cell
            </button>
          </div>

          <div ref={(el) => (sectionRefs.current.upload = el)} class="space-y-4">
            <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wide">Upload cells</h2>
            {uploadCells.map((c) => (
              <DraggableCell
                key={c.id}
                cellRef={(el) => (cellRefs.current[c.seq] = el)}
                dragging={dragState?.kind === 'upload' && dragState.draggedId === c.id}
                onDragStart={() => setDragState({ kind: 'upload', draggedId: c.id })}
                onDragOver={(e) => e.preventDefault()}
                onDrop={() => onDrop('upload', uploadCells, c.id)}
              >
                <Cell
                  cell={c}
                  editCells={editCells}
                  project={project}
                  collapsed={collapsed.has(c.id)}
                  onToggleCollapse={() => toggleCollapsed(c.id)}
                  onChanged={refresh}
                  onDelete={refresh}
                />
              </DraggableCell>
            ))}
            {uploadCells.length === 0 && (
              <p class="text-sm text-slate-500">
                No upload cells yet. Add one from a finished edit cell above once it's done.
              </p>
            )}
          </div>

          <div ref={(el) => (sectionRefs.current.text = el)} class="space-y-4">
            <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wide">Notes</h2>
            {textCells.map((c) => (
              <DraggableCell
                key={c.id}
                cellRef={(el) => (cellRefs.current[c.seq] = el)}
                dragging={dragState?.kind === 'text' && dragState.draggedId === c.id}
                onDragStart={() => setDragState({ kind: 'text', draggedId: c.id })}
                onDragOver={(e) => e.preventDefault()}
                onDrop={() => onDrop('text', textCells, c.id)}
              >
                <Cell
                  cell={c}
                  editCells={editCells}
                  project={project}
                  collapsed={collapsed.has(c.id)}
                  onToggleCollapse={() => toggleCollapsed(c.id)}
                  onChanged={refresh}
                  onDelete={refresh}
                />
              </DraggableCell>
            ))}
            {textCells.length === 0 && (
              <p class="text-sm text-slate-500">No notes yet. Add one to keep links, timestamps, or ideas with this video.</p>
            )}
            <button class="btn-secondary" onClick={addText}>
              <Plus size={15} /> Add note
            </button>
          </div>
        </>
      )}
    </div>
  );
}

function reorderLocal(cells, kind, orderedIds) {
  const byKind = cells.filter((c) => c.kind === kind);
  const others = cells.filter((c) => c.kind !== kind);
  const positions = byKind.map((c) => c.position).sort((a, b) => a - b);
  const reordered = orderedIds.map((id, i) => ({
    ...byKind.find((c) => c.id === id),
    position: positions[i],
  }));
  return [...others, ...reordered].sort((a, b) => a.position - b.position);
}

// DraggableCell only lets a drag start from the grip handle, not anywhere
// in the cell body: `draggable` has to live on this wrapping div for HTML5
// drag-and-drop to work at all, but leaving it permanently true made the
// *entire* cell draggable — every input, button, and bit of selectable
// text inside it — since a browser treats any press-and-move gesture
// inside a draggable ancestor as a drag rather than a click/selection.
// Instead, `draggable` only flips on while the pointer is held down on the
// handle itself (set before the drag actually starts, which HTML5 DnD
// allows), and back off once the drag ends — so everywhere else in the
// cell behaves like normal, selectable, clickable content.
function DraggableCell({ children, cellRef, dragging, onDragStart, onDragOver, onDrop }) {
  const [canDrag, setCanDrag] = useState(false);
  return (
    <div
      ref={cellRef}
      draggable={canDrag}
      onDragStart={onDragStart}
      onDragEnd={() => setCanDrag(false)}
      onDragOver={onDragOver}
      onDrop={onDrop}
      class={`transition-opacity ${dragging ? 'opacity-40' : ''}`}
    >
      <div class="flex items-start gap-1">
        <span
          class="mt-4 cursor-grab active:cursor-grabbing text-slate-600 hover:text-slate-400"
          title="Drag to reorder"
          onMouseDown={() => setCanDrag(true)}
          onMouseUp={() => setCanDrag(false)}
        >
          <GripVertical size={16} />
        </span>
        <div class="flex-1 min-w-0">{children}</div>
      </div>
    </div>
  );
}
