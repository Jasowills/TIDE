import { useEffect, useState } from 'react';
import { TideEvent, fetchEvents, streamEvents } from '../api';

// T101 Live map — vehicle markers from the event stream + state lookups.
// Map provider is pluggable: this SVG renderer implements the MapProvider
// seam; MapLibre replaces it without touching screens (Design §4.2).
export function LiveMap({ tenant }: { tenant: string }) {
  const [positions, setPositions] = useState<Map<string, { lat: number; lng: number; motion: string }>>(new Map());

  useEffect(() => {
    fetchEvents(tenant).then((evs) => {
      const m = new Map(positions);
      for (const e of evs.slice(-200)) {
        const p = e.payload as Record<string, number> | undefined;
        if (p && typeof p['lat'] === 'number') m.set(e.vehicleId, { lat: p['lat'], lng: p['lng'], motion: '' });
      }
      setPositions(m);
    }).catch(() => {});
    return streamEvents((e: TideEvent) => {
      const p = e.payload as Record<string, number> | undefined;
      if (p && typeof p['lat'] === 'number') {
        setPositions((prev) => new Map(prev).set(e.vehicleId, { lat: p['lat'], lng: p['lng'], motion: '' }));
      }
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tenant]);

  const pts = [...positions.entries()];
  const lats = pts.map(([, p]) => p.lat);
  const lngs = pts.map(([, p]) => p.lng);
  const minLat = Math.min(...lats, 0), maxLat = Math.max(...lats, 1);
  const minLng = Math.min(...lngs, 0), maxLng = Math.max(...lngs, 1);
  const X = (lng: number) => 20 + ((lng - minLng) / (maxLng - minLng || 1)) * 760;
  const Y = (lat: number) => 20 + (1 - (lat - minLat) / (maxLat - minLat || 1)) * 380;

  return (
    <div>
      <h2>Live map ({pts.length} vehicles)</h2>
      <svg width={800} height={420} style={{ border: '1px solid #444' }}>
        {pts.map(([id, p]) => (
          <g key={id}>
            <circle cx={X(p.lng)} cy={Y(p.lat)} r={5} fill="#4da3ff" />
            <text x={X(p.lng) + 7} y={Y(p.lat) + 4} fontSize={10} fill="#ccc">{id}</text>
          </g>
        ))}
      </svg>
    </div>
  );
}
