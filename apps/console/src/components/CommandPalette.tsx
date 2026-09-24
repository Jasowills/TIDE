import { useEffect, useState } from 'react';

// Command palette: keyboard-first navigation for the whole console. Sections,
// the simulator, and every known vehicle are one keystroke away. Rendered
// fixed so it escapes every scroll container.
export interface PaletteCommand {
  id: string;
  group: string;
  label: string;
  hint?: string;
  run: () => void;
}

export function CommandPalette({ open, onClose, commands }: {
  open: boolean;
  onClose: () => void;
  commands: PaletteCommand[];
}) {
  const [q, setQ] = useState('');
  const [index, setIndex] = useState(0);

  useEffect(() => {
    if (open) {
      setQ('');
      setIndex(0);
    }
  }, [open ]);

  if (!open) return null;

  const needle = q.trim().toLowerCase();
  const filtered = needle
    ? commands.filter((c) => `${c.group} ${c.label}`.toLowerCase().includes(needle))
    : commands;
  const cursor = Math.min(index, Math.max(0, filtered.length - 1));

  function onKey(e: React.KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setIndex((i) => Math.min(i + 1, filtered.length - 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setIndex((i) => Math.max(i - 1, 0));
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const cmd = filtered[cursor];
      if (cmd) {
        cmd.run();
        onClose();
      }
    } else if (e.key === 'Escape') {
      e.preventDefault();
      onClose();
    }
  }

  let lastGroup = '';
  return (
    <div className="palette-backdrop" onClick={onClose}>
      <div
        className="palette"
        role="dialog"
        aria-modal="true"
        aria-label="Command Palette"
        onClick={(e) => e.stopPropagation()}
      >
        <input
          autoFocus
          value={q}
          onChange={(e) => {
            setQ(e.target.value);
            setIndex(0);
          }}
          onKeyDown={onKey}
          placeholder="Type a Section or Vehicle ID"
          aria-label="Command Palette"
        />
        <ul className="palette-list">
          {filtered.map((c, i) => {
            const head = c.group !== lastGroup ? c.group : null;
            lastGroup = c.group;
            return (
              <li key={c.id}>
                {head && <div className="palette-group">{head}</div>}
                <div className={`palette-item${i === cursor ? ' active' : ''}`}>
                  <button
                    onClick={() => {
                      c.run();
                      onClose();
                    }}
                    onMouseEnter={() => setIndex(i)}
                  >
                    {c.label}
                    {c.hint && <span className="hint">{c.hint}</span>}
                  </button>
                </div>
              </li>
            );
          })}
          {filtered.length === 0 && (
            <li className="palette-group">No Matching Commands or Vehicles</li>
          )}
        </ul>
        <div className="palette-foot">↑↓ Navigate · Enter Open · Esc Close</div>
      </div>
    </div>
  );
}
