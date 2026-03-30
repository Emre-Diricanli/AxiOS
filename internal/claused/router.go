package claused

import "log/slog"

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
	mode          RoutingMode
	cloudAvail    bool
	localAvail    bool
	logger        *slog.Logger
}

// NewRouter creates a new model router.
func NewRouter(mode RoutingMode, logger *slog.Logger) *Router {
	return &Router{
		mode:   mode,
		logger: logger,
	}
}

// SetCloudAvailable updates cloud backend availability.
func (r *Router) SetCloudAvailable(available bool) {
	r.cloudAvail = available
}

// SetLocalAvailable updates local model backend availability.
func (r *Router) SetLocalAvailable(available bool) {
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
