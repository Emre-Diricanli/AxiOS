package axiosd

import (
	"sync"
	"testing"
)

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

func TestRouterConcurrentSetModeAndRoute(t *testing.T) {
	r := NewRouter(RouteAuto, testLogger())
	r.SetCloudAvailable(true)
	r.SetLocalAvailable(true)

	modes := []RoutingMode{RouteAuto, RouteCloudOnly, RouteLocalOnly, RouteCostAware}
	const (
		goroutines = 8
		iterations = 10000
	)

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			for j := 0; j < iterations; j++ {
				if worker%2 == 0 {
					r.SetMode(modes[(worker+j)%len(modes)])
					continue
				}
				got := r.Route()
				if got != BackendCloud && got != BackendLocal {
					t.Errorf("Route() = %q, want a known backend", got)
					return
				}
			}
		}(i)
	}

	close(start)
	wg.Wait()

	r.SetMode(RouteLocalOnly)
	if got := r.Mode(); got != RouteLocalOnly {
		t.Errorf("Mode() = %q, want %q", got, RouteLocalOnly)
	}
}
