package claused

import (
	"log/slog"

	"github.com/axios-os/axios/pkg/permissions"
)

// PermissionEnforcer checks operations against the permission model.
type PermissionEnforcer struct {
	config *permissions.Config
	logger *slog.Logger
}

// NewPermissionEnforcer creates a new permission enforcer from config.
func NewPermissionEnforcer(config *permissions.Config, logger *slog.Logger) *PermissionEnforcer {
	return &PermissionEnforcer{
		config: config,
		logger: logger,
	}
}

// CanExecute checks if an operation is allowed without user approval.
func (pe *PermissionEnforcer) CanExecute(operation string) bool {
	tier := pe.config.Check(operation)
	pe.logger.Debug("permission check", "operation", operation, "tier", tier)
	return tier == permissions.Trusted
}

// NeedsApproval checks if an operation requires user confirmation.
func (pe *PermissionEnforcer) NeedsApproval(operation string) bool {
	return pe.config.Check(operation) == permissions.ApprovalRequired
}

// IsProhibited checks if an operation is forbidden.
func (pe *PermissionEnforcer) IsProhibited(operation string) bool {
	return pe.config.Check(operation) == permissions.Prohibited
}
