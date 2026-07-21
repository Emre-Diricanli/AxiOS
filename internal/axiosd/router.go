package axiosd

import (
	"log/slog"
	"sync"
)

// RoutingMode determines how requests are routed between AI backends.
type RoutingMode string

const (
	RouteAuto      RoutingMode = "auto"
	RouteCloudOnly RoutingMode = "cloud_only"
	RouteLocalOnly RoutingMode = "local_only"
	RouteCostAware RoutingMode = "cost_aware"
)

// Router decides which AI backend handles a given request.
type Router struct {
	mode       RoutingMode
	cloudAvail bool
	localAvail bool
	logger     *slog.Logger
	mu         sync.RWMutex
}

// NewRouter creates a new model router.
func NewRouter(mode RoutingMode, logger *slog.Logger) *Router {
	return &Router{
		mode:   mode,
		logger: logger,
	}
}

// SetMode updates the routing mode.
func (r *Router) SetMode(mode RoutingMode) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mode = mode
}

// Mode returns the current routing mode.
func (r *Router) Mode() RoutingMode {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mode
}

// SetCloudAvailable updates cloud backend availability.
func (r *Router) SetCloudAvailable(available bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cloudAvail = available
}

// SetLocalAvailable updates local model backend availability.
func (r *Router) SetLocalAvailable(available bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.localAvail = available
}

// Backend represents which AI backend to use.
type Backend string

const (
	BackendCloud Backend = "cloud"
	BackendLocal Backend = "local"
)

// Route decides which backend should handle a request.
func (r *Router) Route() Backend {
	r.mu.RLock()
	defer r.mu.RUnlock()

	switch r.mode {
	case RouteCloudOnly:
		return BackendCloud
	case RouteLocalOnly:
		return BackendLocal
	case RouteCostAware:
		// TODO: implement budget tracking
		if r.cloudAvail {
			return BackendCloud
		}
		return BackendLocal
	default: // auto
		if r.cloudAvail {
			return BackendCloud
		}
		if r.localAvail {
			r.logger.Info("cloud unavailable, falling back to local model")
			return BackendLocal
		}
		// Default to cloud and let it fail with a clear error
		return BackendCloud
	}
}
