package tenants

import (
	"context"
	"testing"
)

// T011 acceptance: one vehicle holds 2+ provider identities without collision,
// and resolution is tenant-scoped.
func TestMultipleIdentities(t *testing.T) {
	ctx := context.Background()
	r := NewMemoryRepository()
	_ = r.CreateTenant(ctx, Tenant{ID: "t1"})
	_ = r.CreateVehicle(ctx, Vehicle{ID: "v1", TenantID: "t1"})
	_ = r.AddIdentity(ctx, "t1", ProviderIdentity{VehicleID: "v1", Provider: "traccar", ProviderID: "42"})
	_ = r.AddIdentity(ctx, "t1", ProviderIdentity{VehicleID: "v1", Provider: "flespi", ProviderID: "9f8"})

	ids, _ := r.Identities(ctx, "t1", "v1")
	if len(ids) != 2 {
		t.Fatalf("want 2 identities, got %d", len(ids))
	}
	for _, p := range [][2]string{{"traccar", "42"}, {"flespi", "9f8"}} {
		v, err := r.ResolveVehicle(ctx, "t1", p[0], p[1])
		if err != nil || v.ID != "v1" {
			t.Fatalf("resolve %v: %v %+v", p, err, v)
		}
	}
	// Same provider id in another tenant must NOT resolve here.
	_ = r.CreateTenant(ctx, Tenant{ID: "t2"})
	if _, err := r.ResolveVehicle(ctx, "t2", "traccar", "42"); err == nil {
		t.Fatal("cross-tenant resolution must fail")
	}
}
