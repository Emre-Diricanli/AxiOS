package axiosd

import "testing"

func TestRouterModes(t *testing.T) {
	tests := []struct {
		name       string
		mode       RoutingMode
		cloudAvail bool
		localAvail bool
		want       Backend
	}{
		{"cloud_only always cloud", RouteCloudOnly, false, true, BackendCloud},
		{"local_only always local", RouteLocalOnly, true, false, BackendLocal},
		{"auto prefers cloud", RouteAuto, true, true, BackendCloud},
		{"auto falls back to local", RouteAuto, false, true, BackendLocal},
		{"auto with nothing defaults to cloud", RouteAuto, false, false, BackendCloud},
		{"cost_aware uses cloud when available", RouteCostAware, true, true, BackendCloud},
		{"cost_aware falls back to local", RouteCostAware, false, true, BackendLocal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRouter(tt.mode, testLogger())
			r.SetCloudAvailable(tt.cloudAvail)
			r.SetLocalAvailable(tt.localAvail)
			if got := r.Route(); got != tt.want {
				t.Errorf("Route() = %q, want %q", got, tt.want)
			}
		})
	}
}
