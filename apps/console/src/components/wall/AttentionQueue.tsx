import type { TideEvent } from '../../api';
import { clockOf, getIncidents } from './model';

// Attention queue: every open incident in the current log, newest first.
// This is the dispatcher's inbox — the one place that answers "what needs
// action right now". Selecting a row loads the vehicle into the dossier.
export function AttentionQueue({ events, selected, onSelect }: {
  events: TideEvent[];
  selected: string | null;
  onSelect: (v: string) => void;
}) {
  const items = getIncidents(events, 30);

  return (
    <section className="panel" aria-label="Attention Queue">
      <div className="panel-h">
        <h3>Attention Queue</h3>
        {items.length > 0 && <span className="pill bad">{items.length} Open</span>}
      </div>
      {items.length === 0 ? (
        <p className="panel-empty">Queue Clear. No Open Incidents in the Current Log.</p>
      ) : (
        <ul className="queue-list">
          {items.map((e) => (
            <li key={e.id} className={e.vehicleId === selected ? 'selected' : undefined}>
              <button onClick={() => onSelect(e.vehicleId)} aria-label={`${e.type}, ${e.vehicleId}`}>
                <span className="t">{clockOf(e.timestamp)}</span>
                <span className="q-main">
                  <span className="q-type">{e.type}</span>
                  <span className="q-veh">{e.vehicleId}</span>
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
