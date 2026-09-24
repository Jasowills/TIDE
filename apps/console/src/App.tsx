import { Fragment, useEffect, useMemo, useState } from 'react';
import { Activity, FileSearch, FlaskConical } from 'lucide-react';
import { Pipeline } from './views/Pipeline';
import { Inspect } from './views/Inspect';
import { Simulate } from './views/Simulate';
import { CommandPalette, type PaletteCommand } from './components/CommandPalette';
import { StatusBar } from './components/StatusBar';
import { fetchConnections, fetchEvents } from './api';
import { screens, scrollToSection, sectionAnchor, useTideNav, type Screen } from './nav';

// Console frame: a slim command bar, one sticky section nav, a horizontal
// pipeline strip, and a single page ordered the way data flows — pipe, then
// meaning, then source. A fixed status bar holds the live readout; ⌘K opens
// the command palette from anywhere.
const blurbs: Record<Screen, string> = {
  Pipeline: 'Live status, events, throughput, and adapters',
  Inspect: 'Vehicle state, history, and rule triggers',
  Simulate: 'FleetSim traffic and fault injection',
};

function Icon({ name }: { name: Screen }) {
  const props = { size: 15, strokeWidth: 2, 'aria-hidden': true } as const;
  switch (name) {
    case 'Pipeline': return <Activity {...props} />;
    case 'Inspect': return <FileSearch {...props} />;
    case 'Simulate': return <FlaskConical {...props} />;
  }
}

const STAGES = [
  { key: 'ingest', nm: 'Ingest', ds: 'MQTT, HTTP, Traccar' },
  { key: 'validate', nm: 'Validate', ds: 'Schema and Units' },
  { key: 'dedup', nm: 'Deduplicate', ds: 'Sequence First' },
  { key: 'state', nm: 'State', ds: 'Motion, Presence' },
  { key: 'rules', nm: 'Rules', ds: 'Versioned, Cooldown' },
  { key: 'events', nm: 'Events', ds: 'Started, Ended' },
  { key: 'webhooks', nm: 'Webhooks', ds: 'Signed Delivery' },
];

function stageClass(key: string): string {
  if (key === 'ingest' || key === 'events') return 'hot';
  if (key === 'rules') return 'rule';
  if (key === 'webhooks') return 'sink';
  return '';
}

export function App() {
  const nav = useTideNav();
  const { screen, setScreen, tenant, setTenant } = nav;
  const [counts, setCounts] = useState({ vehicles: 0, events: 0, incidents: 0, adapters: 0, healthy: 0 });
  const [vehicleIds, setVehicleIds] = useState<string[]>([]);
  const [apiOk, setApiOk] = useState(true);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [active, setActive] = useState<Screen>(screen);

  useEffect(() => {
    let dead = false;
    async function load() {
      try {
        const [evs, conns] = await Promise.all([
          fetchEvents(tenant).catch(() => []),
          fetchConnections().catch(() => []),
        ]);
        if (dead) return;
        setCounts({
          vehicles: new Set(evs.map((e) => e.vehicleId)).size,
          events: evs.length,
          incidents: evs.filter((e) => e.type.startsWith('incident.')).length,
          adapters: conns.length,
          healthy: conns.filter((c) => c.state === 'HEALTHY').length,
        });
        setVehicleIds([...new Set(evs.map((e) => e.vehicleId))].sort());
        setApiOk(true);
      } catch {
        if (!dead) setApiOk(false);
      }
    }
    load();
    const t = setInterval(load, 10_000);
    return () => { dead = true; clearInterval(t); };
  }, [tenant]);

  // Command-K opens the palette from anywhere outside form fields.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement)?.tagName;
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setPaletteOpen((v) => !v);
      } else if (tag === 'INPUT' || tag === 'SELECT' || tag === 'TEXTAREA') {
        return;
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  // Scroll-spy: the sticky nav tracks the section in view.
  useEffect(() => {
    const obs = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          if (!e.isIntersecting) continue;
          const match = screens.find((s) => sectionAnchor(s) === (e.target as HTMLElement).id);
          if (match) setActive(match);
        }
      },
      { rootMargin: '-35% 0px -55% 0px' },
    );
    screens.forEach((s) => {
      const el = document.getElementById(sectionAnchor(s));
      if (el) obs.observe(el);
    });
    return () => obs.disconnect();
  }, []);

  function stageCount(key: string): string {
    switch (key) {
      case 'ingest': return String(counts.adapters || '—');
      case 'state': return String(counts.vehicles || '—');
      case 'events': return String(counts.events || '—');
      case 'rules': return String(counts.incidents || '—');
      default: return '·';
    }
  }

  function go(s: Screen) {
    setScreen(s);
    setActive(s);
  }

  const commands: PaletteCommand[] = useMemo(
    () => [
      ...screens.map((s) => ({
        id: `go-${s}`,
        group: 'Sections',
        label: `Go to ${s}`,
        run: () => go(s),
      })),
      {
        id: 'go-sim-run',
        group: 'Sections',
        label: 'Run Traffic Simulation',
        run: () => go('Simulate'),
      },
      ...vehicleIds.map((v) => ({
        id: `veh-${v}`,
        group: 'Vehicles',
        label: v,
        run: () => nav.openVehicle(v),
      })),
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [vehicleIds, screen],
  );

  return (
    <div className="deck">
      <header className="cmdbar">
        <h1 className="brand-stamp" style={{ margin: 0, fontSize: 16 }}>
          <span className="brand-mark" aria-hidden>T</span>
          <span>TIDE Console
            <span className="brand-sub"> Fleet Command</span>
          </span>
        </h1>
        <div className="cmdbar-right">
          <label className="tenant-chip">
            Tenant
            <input value={tenant} onChange={(e) => setTenant(e.target.value)} aria-label="Tenant" />
          </label>
        </div>
      </header>

      <nav className="secnav" aria-label="Console Sections">
        {screens.map((s, i) => (
          <button
            key={s}
            onClick={() => go(s)}
            className={active === s ? 'active' : undefined}
            aria-current={active === s ? 'true' : undefined}
            title={`${blurbs[s]} (Key ${i + 1})`}
          >
            <Icon name={s} />
            {s}
            <span className="kbd" aria-hidden>{i + 1}</span>
          </button>
        ))}
        <div className="secnav-right">
          <button className="btn ghost sm" onClick={() => setPaletteOpen(true)} title="Command Palette (Cmd+K)">
            ⌘K Commands
          </button>
        </div>
      </nav>

      <main className="workspace">
        <section className="console-section" id={sectionAnchor('Pipeline')} aria-label="Pipeline">
          <div className="pipestrip" role="status" aria-label="Pipeline Stages With Live Counts">
            {STAGES.map((s, i) => (
              <Fragment key={s.key}>
                <div className={`pipebit ${stageClass(s.key)}`} title={`${s.nm} — ${s.ds}`}>
                  <span className="pipebit-n">{i + 1}</span>
                  <span className="pipebit-tx">
                    <div className="nm">{s.nm}</div>
                    <div className="ds">{s.ds}</div>
                  </span>
                  <span className="ct">{stageCount(s.key)}</span>
                </div>
                {i < STAGES.length - 1 && <span className="pipebit-arrow" aria-hidden>→</span>}
              </Fragment>
            ))}
          </div>
          <Pipeline tenant={tenant} onOpenVehicle={nav.openVehicle} onOpenReplay={nav.openReplay} />
        </section>
        <section className="console-section" id={sectionAnchor('Inspect')} aria-label="Inspect">
          <Inspect tenant={tenant} initialVehicle={nav.focusVehicle} onOpenReplay={nav.openReplay} />
        </section>
        <section className="console-section" id={sectionAnchor('Simulate')} aria-label="Simulate">
          <Simulate tenant={tenant} onViewOutput={nav.openExplorer} />
        </section>
      </main>

      <footer className="console-foot">
        <span><strong>TIDE Console.</strong> Fleet Command for Pipeline Operators.</span>
        <span>Debug Instrument. No Dispatch, Scheduling, or Payroll.</span>
        <span className="mono" style={{ marginLeft: 'auto' }}>tide doctor · tide simulate · tide replay</span>
      </footer>

      <CommandPalette open={paletteOpen} onClose={() => setPaletteOpen(false)} commands={commands} />
      <StatusBar
        tenant={tenant}
        vehicles={counts.vehicles}
        events={counts.events}
        incidents={counts.incidents}
        adapters={counts.adapters}
        healthy={counts.healthy}
        apiOk={apiOk}
      />
    </div>
  );
}
