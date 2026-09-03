// Package store is durable Postgres storage for telemetry + events.
// Tables are partition-ready (tenant_id + ts indexes, ADR-001 §3).
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
	"github.com/tide-telematics/tide/schemas/events"
)

type PG struct {
	db *sql.DB
}

func Open(ctx context.Context, dsn string) (*PG, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	return &PG{db: db}, nil
}

func (p *PG) AppendTelemetry(ctx context.Context, t ctelemetry.Telemetry) error {
	payload, _ := json.Marshal(map[string]any{
		"speedKmh": t.SpeedKmh, "ignition": t.Ignition, "location": t.Location,
		"engine": t.Engine, "sensors": t.Sensors, "source": t.Source, "metadata": t.Metadata,
	})
	raw, _ := json.Marshal(t.Raw)
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO telemetry(id, tenant_id, vehicle_id, device_id, ts, received_at, processed_at, lat, lng, speed_kmh, payload, raw, dedup_key)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (tenant_id, dedup_key) DO NOTHING`,
		t.ID, t.TenantID, t.VehicleID, t.DeviceID, t.Timestamp, t.ReceivedAt, t.ProcessedAt,
		t.Location.Lat, t.Location.Lng, t.SpeedKmh, payload, raw, t.DedupKey())
	if err != nil {
		return fmt.Errorf("store: append telemetry: %w", err)
	}
	return nil
}

func (p *PG) AppendEvent(ctx context.Context, e events.Event) error {
	payload, _ := json.Marshal(e.Payload)
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO events(id, "type", tenant_id, vehicle_id, ts, rule_id, rule_version, correlation_id, causation_id, payload, schema_version)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT (id) DO NOTHING`,
		e.ID, e.Type, e.TenantID, e.VehicleID, e.Timestamp, e.RuleID, e.RuleVersion,
		e.CorrelationID, e.CausationID, payload, e.SchemaVersion)
	return err
}

// RecentEvents serves the console Event explorer (tenant-scoped, §2.11).
func (p *PG) RecentEvents(ctx context.Context, tenantID, vehicleID, typ string, limit int) ([]events.Event, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("store: tenant required")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT id, "type", tenant_id, vehicle_id, ts, rule_id, rule_version, correlation_id, causation_id, payload, schema_version
	      FROM events WHERE tenant_id = $1`
	args := []any{tenantID}
	if vehicleID != "" {
		q += fmt.Sprintf(` AND vehicle_id = $%d`, len(args)+1)
		args = append(args, vehicleID)
	}
	if typ != "" {
		q += fmt.Sprintf(` AND "type" = $%d`, len(args)+1)
		args = append(args, typ)
	}
	q += fmt.Sprintf(` ORDER BY ts DESC LIMIT $%d`, len(args)+1)
	args = append(args, limit)
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []events.Event
	for rows.Next() {
		var e events.Event
		var payload []byte
		var ruleID, ruleVer, causation sql.NullString
		if err := rows.Scan(&e.ID, &e.Type, &e.TenantID, &e.VehicleID, &e.Timestamp,
			&ruleID, &ruleVer, &e.CorrelationID, &causation, &payload, &e.SchemaVersion); err != nil {
			return nil, err
		}
		e.RuleID, e.RuleVersion, e.CausationID = ruleID.String, ruleVer.String, causation.String
		_ = json.Unmarshal(payload, &e.Payload)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (p *PG) Close() error { return p.db.Close() }

// AddGeofence persists a geofence polygon (lon/lat ring) as PostGIS geometry.
func (p *PG) AddGeofence(ctx context.Context, tenantID, id, name string, ring [][2]float64) error {
	if tenantID == "" || id == "" || len(ring) < 3 {
		return fmt.Errorf("store: geofence needs tenant, id, 3+ points")
	}
	pts := ""
	for i, pt := range ring {
		if i > 0 {
			pts += ","
		}
		pts += fmt.Sprintf("%f %f", pt[0], pt[1])
	}
	pts += "," + fmt.Sprintf("%f %f", ring[0][0], ring[0][1])
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO geofences(id, tenant_id, name, geom)
		 VALUES ($1,$2,$3,ST_GeomFromText('POLYGON(('||$4||'))',4326))
		 ON CONFLICT (id) DO NOTHING`, id, tenantID, name, pts)
	return err
}
