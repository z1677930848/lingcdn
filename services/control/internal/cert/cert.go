package cert

import "context"

// Manager is reserved for future centralized cert distribution helpers.
// Certificate CRUD and ACME issuance are implemented in internal/server
// (handlers_certificates.go) and persisted via store.
type Manager struct{}

func New() *Manager { return &Manager{} }

// Store is a no-op placeholder kept for dependency injection compatibility.
func (m *Manager) Store(_ context.Context, domain string, certPEM, keyPEM []byte) error {
	_ = domain
	_ = certPEM
	_ = keyPEM
	return nil
}
