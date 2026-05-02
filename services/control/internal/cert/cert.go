package cert

import "context"

// Manager handles certificate storage and distribution (placeholder).
type Manager struct{}

func New() *Manager { return &Manager{} }

// Store is a stub for saving certs.
func (m *Manager) Store(_ context.Context, domain string, certPEM, keyPEM []byte) error {
	_ = domain
	_ = certPEM
	_ = keyPEM
	return nil
}
