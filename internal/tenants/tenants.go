// Package tenants owns Tenant → Fleet → Vehicle → Device → ProviderIdentity.
// Key invariant (§2.2): a Vehicle maps to MULTIPLE provider identities
// (Traccar device id ≠ flespi device id ≠ TIDE vehicle id). Provider ids are
// never assumed globally unique. All queries are tenant-scoped at the repo
// layer — never trust a tenantId from an untrusted request body (§2.11).
package tenants

import (
	"context"
	"fmt"
	"sync"
)

type Tenant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Fleet struct {
	ID       string `json:"id"`
	TenantID string `json:"tenantId"`
	Name     string `json:"name"`
}

type Vehicle struct {
	ID       string `json:"id"`
	TenantID string `json:"tenantId"`
	FleetID  string `json:"fleetId"`
	Name     string `json:"name"`
	// ExpectedCadenceSecs: per-device reporting cadence for offline detection
	// (§42: never a global threshold). 0 = use default 300s.
	ExpectedCadenceSecs int `json:"expectedCadenceSecs"`
}

type Device struct {
	ID        string `json:"id"`
	TenantID  string `json:"tenantId"`
	VehicleID string `json:"vehicleId"`
	Name      string `json:"name"`
}

// ProviderIdentity maps one vehicle to one provider-side id. A vehicle holds
// 2+ of these without collision (T011 acceptance).
type ProviderIdentity struct {
	VehicleID  string `json:"vehicleId"`
	Provider   string `json:"provider"`
	ProviderID string `json:"providerId"`
}

// Repository is the tenant-scoped persistence contract. MemoryRepository is
// the default for tests/dev; PostgresRepository backs prod.
type Repository interface {
	CreateTenant(ctx context.Context, t Tenant) error
	CreateFleet(ctx context.Context, f Fleet) error
	CreateVehicle(ctx context.Context, v Vehicle) error
	CreateDevice(ctx context.Context, d Device) error
	AddIdentity(ctx context.Context, tenantID string, id ProviderIdentity) error
	// ResolveVehicle maps (provider, providerID) → vehicle within a tenant.
	ResolveVehicle(ctx context.Context, tenantID, provider, providerID string) (Vehicle, error)
	Identities(ctx context.Context, tenantID, vehicleID string) ([]ProviderIdentity, error)
	Vehicle(ctx context.Context, tenantID, vehicleID string) (Vehicle, error)
}

type MemoryRepository struct {
	mu         sync.RWMutex
	tenants    map[string]Tenant
	fleets     map[string]Fleet
	vehicles   map[string]Vehicle // key tenantID/vehicleID
	devices    map[string]Device
	identities map[string][]ProviderIdentity // key tenantID/vehicleID
	byProvider map[string]Vehicle             // key tenantID/provider/providerID
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		tenants: map[string]Tenant{}, fleets: map[string]Fleet{},
		vehicles: map[string]Vehicle{}, devices: map[string]Device{},
		identities: map[string][]ProviderIdentity{}, byProvider: map[string]Vehicle{},
	}
}

func vk(tenantID, id string) string { return tenantID + "/" + id }

func (m *MemoryRepository) CreateTenant(_ context.Context, t Tenant) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t.ID == "" {
		return fmt.Errorf("tenant id required")
	}
	m.tenants[t.ID] = t
	return nil
}

func (m *MemoryRepository) CreateFleet(_ context.Context, f Fleet) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if f.TenantID == "" || f.ID == "" {
		return fmt.Errorf("fleet tenantId+id required")
	}
	if _, ok := m.tenants[f.TenantID]; !ok {
		return fmt.Errorf("unknown tenant %q", f.TenantID)
	}
	m.fleets[vk(f.TenantID, f.ID)] = f
	return nil
}

func (m *MemoryRepository) CreateVehicle(_ context.Context, v Vehicle) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v.TenantID == "" || v.ID == "" {
		return fmt.Errorf("vehicle tenantId+id required")
	}
	if _, ok := m.tenants[v.TenantID]; !ok {
		return fmt.Errorf("unknown tenant %q", v.TenantID)
	}
	m.vehicles[vk(v.TenantID, v.ID)] = v
	return nil
}

func (m *MemoryRepository) CreateDevice(_ context.Context, d Device) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d.TenantID == "" || d.ID == "" || d.VehicleID == "" {
		return fmt.Errorf("device tenantId+id+vehicleId required")
	}
	m.devices[vk(d.TenantID, d.ID)] = d
	return nil
}

func (m *MemoryRepository) AddIdentity(_ context.Context, tenantID string, id ProviderIdentity) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if tenantID == "" || id.VehicleID == "" || id.Provider == "" || id.ProviderID == "" {
		return fmt.Errorf("identity requires tenant, vehicle, provider, providerId")
	}
	k := vk(tenantID, id.VehicleID)
	if _, ok := m.vehicles[k]; !ok {
		return fmt.Errorf("unknown vehicle %q in tenant %q", id.VehicleID, tenantID)
	}
	m.identities[k] = append(m.identities[k], id)
	m.byProvider[tenantID+"/"+id.Provider+"/"+id.ProviderID] = m.vehicles[k]
	return nil
}

func (m *MemoryRepository) ResolveVehicle(_ context.Context, tenantID, provider, providerID string) (Vehicle, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.byProvider[tenantID+"/"+provider+"/"+providerID]
	if !ok {
		return Vehicle{}, fmt.Errorf("no vehicle for %s/%s in tenant %s", provider, providerID, tenantID)
	}
	return v, nil
}

func (m *MemoryRepository) Identities(_ context.Context, tenantID, vehicleID string) ([]ProviderIdentity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]ProviderIdentity{}, m.identities[vk(tenantID, vehicleID)]...), nil
}

func (m *MemoryRepository) Vehicle(_ context.Context, tenantID, vehicleID string) (Vehicle, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.vehicles[vk(tenantID, vehicleID)]
	if !ok {
		return Vehicle{}, fmt.Errorf("unknown vehicle %q", vehicleID)
	}
	return v, nil
}
