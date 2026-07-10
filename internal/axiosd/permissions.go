package axiosd

import (
	"crypto/rand"
	"encoding/hex"
	"sync"

	"github.com/axios-os/axios/pkg/permissions"
)

// PermissionChecker decides the trust tier of a model-initiated tool call.
// *permissions.Config satisfies it; tests supply fakes.
type PermissionChecker interface {
	Check(toolName string, args map[string]any) permissions.Tier
}

var _ PermissionChecker = (*permissions.Config)(nil)

// approvalRegistry tracks in-flight approval requests: a mutexed map keyed
// by request id with one buffered channel per pending entry. The websocket
// read loop resolves entries when {"type":"approval_response"} arrives;
// executeTool blocks on the channel until then (or its timeout).
type approvalRegistry struct {
	mu      sync.Mutex
	pending map[string]chan bool
}

// register creates a pending entry and returns the channel the waiter
// blocks on. The channel is buffered so a resolver never blocks even if
// the waiter has already timed out.
func (r *approvalRegistry) register(id string) chan bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pending == nil {
		r.pending = make(map[string]chan bool)
	}
	ch := make(chan bool, 1)
	r.pending[id] = ch
	return ch
}

// resolve delivers the user's verdict to the pending waiter. It reports
// whether a pending entry existed for the id.
func (r *approvalRegistry) resolve(id string, approve bool) bool {
	r.mu.Lock()
	ch, ok := r.pending[id]
	delete(r.pending, id)
	r.mu.Unlock()
	if !ok {
		return false
	}
	ch <- approve // buffered; never blocks
	return true
}

// remove discards a pending entry (waiter timed out or was canceled).
func (r *approvalRegistry) remove(id string) {
	r.mu.Lock()
	delete(r.pending, id)
	r.mu.Unlock()
}

// newApprovalID returns a random correlation id for an approval request.
func newApprovalID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand.Read only fails when the OS entropy source is broken;
		// the id is a correlation token, not a secret, so any error here is
		// surfaced as a process-level problem.
		panic("axiosd: crypto/rand failed: " + err.Error())
	}
	return "appr-" + hex.EncodeToString(b[:])
}
