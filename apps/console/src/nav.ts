import { useCallback, useEffect, useState } from 'react';

// Single-page console. All three sections live on one scrollable page,
// ordered the way data flows through TIDE: pipe, then meaning, then source.
// "Navigation" scrolls to a section; the URL still records it (?screen=) so
// every section deep-links. Legacy ?screen=Command and ?screen=Lab URLs
// resolve to the renamed sections.
export const screens = ['Pipeline', 'Inspect', 'Simulate'] as const;
export type Screen = (typeof screens)[number];

const LEGACY: Record<string, Screen> = {
  Command: 'Pipeline',
  Lab: 'Simulate',
};

const ANCHORS: Record<Screen, string> = {
  Pipeline: 's-pipeline',
  Inspect: 's-inspect',
  Simulate: 's-simulate',
};

export function sectionAnchor(s: Screen): string {
  return ANCHORS[s];
}

export function scrollToSection(s: Screen) {
  document.getElementById(ANCHORS[s])?.scrollIntoView({ behavior: 'smooth', block: 'start' });
}

function readURL(): { screen: Screen; tenant: string; vehicle: string; event: string } {
  const q = new URLSearchParams(window.location.search);
  const raw = q.get('screen');
  const s = (raw && (LEGACY[raw] ?? (raw as Screen))) as Screen | null;
  return {
    screen: s && (screens as readonly string[]).includes(s) ? s : 'Pipeline',
    tenant: q.get('tenant') || 'default',
    vehicle: q.get('vehicle') || '',
    event: q.get('event') || '',
  };
}

export function useTideNav() {
  const [initial] = useState(readURL);
  const [screen, setScreenState] = useState<Screen>(initial.screen);
  const [tenant, setTenantState] = useState(initial.tenant);
  const [focusVehicle, setFocusVehicle] = useState(initial.vehicle);
  const [focusEvent, setFocusEvent] = useState(initial.event);
  const [streamState, setStreamState] = useState<'live' | 'paused' | 'stale'>('live');

  const sync = useCallback((s: Screen, t: string, v: string, e: string) => {
    const q = new URLSearchParams();
    if (s !== 'Pipeline') q.set('screen', s);
    if (t && t !== 'default') q.set('tenant', t);
    if (v) q.set('vehicle', v);
    if (e) q.set('event', e);
    const qs = q.toString();
    window.history.replaceState(null, '', qs ? `?${qs}` : window.location.pathname);
  }, []);

  useEffect(() => {
    sync(screen, tenant, focusVehicle, focusEvent);
  }, [screen, tenant, focusVehicle, focusEvent, sync]);

  const setScreen = useCallback((s: Screen) => {
    setScreenState(s);
    scrollToSection(s);
  }, []);

  const setTenant = useCallback((t: string) => setTenantState(t), []);

  const openVehicle = useCallback((vehicle: string) => {
    setFocusVehicle(vehicle);
    setScreenState('Inspect');
    scrollToSection('Inspect');
  }, []);

  const openReplay = useCallback((vehicle: string, event?: string) => {
    setFocusVehicle(vehicle);
    if (event) setFocusEvent(event);
    setScreenState('Inspect');
    scrollToSection('Inspect');
  }, []);

  const openExplorer = useCallback((vehicle?: string) => {
    if (vehicle) setFocusVehicle(vehicle);
    setScreenState('Pipeline');
    scrollToSection('Pipeline');
  }, []);

  // Deep links land on their section once the page has mounted.
  useEffect(() => {
    if (initial.screen === 'Pipeline') return;
    const t = setTimeout(() => scrollToSection(initial.screen), 150);
    return () => clearTimeout(t);
  }, [initial.screen]);

  // Keys 1-3 jump to sections from anywhere outside form fields.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement)?.tagName;
      if (tag === 'INPUT' || tag === 'SELECT' || tag === 'TEXTAREA') return;
      const n = Number(e.key);
      if (n >= 1 && n <= 3) {
        const s = screens[n - 1];
        setScreenState(s);
        scrollToSection(s);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  return { screen, setScreen, tenant, setTenant, focusVehicle, setFocusVehicle, focusEvent, setFocusEvent, openVehicle, openReplay, openExplorer, streamState, setStreamState };
}
