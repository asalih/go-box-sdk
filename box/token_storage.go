// Package box contains the authentication methods (CCG, JWT, OAuth2, Developer
// Token), token storage, and a slim BoxClient that wires the ported managers
// onto a NetworkSession. It mirrors the relevant files under src/box in the
// Box SDK.
package box

import (
	"sync"

	"github.com/asalih/go-box-sdk/schemas"
)

// TokenStorage stores and retrieves the cached access token. It mirrors the
// TokenStorage interface in src/box/tokenStorage.ts. The async source methods
// map to synchronous calls here since no storage backend performs I/O in the
// default implementation.
type TokenStorage interface {
	// Store caches the given token.
	Store(token *schemas.AccessToken)
	// Get returns the cached token, or nil when none is stored.
	Get() *schemas.AccessToken
	// Clear discards the cached token.
	Clear()
}

// InMemoryTokenStorage keeps the token in memory. It mirrors
// InMemoryTokenStorage and is safe for concurrent use.
type InMemoryTokenStorage struct {
	mu    sync.Mutex
	token *schemas.AccessToken
}

// NewInMemoryTokenStorage returns an empty in-memory token storage.
func NewInMemoryTokenStorage() *InMemoryTokenStorage {
	return &InMemoryTokenStorage{}
}

// NewInMemoryTokenStorageWithToken returns an in-memory storage seeded with the
// given token, mirroring the developer-token construction path.
func NewInMemoryTokenStorageWithToken(token *schemas.AccessToken) *InMemoryTokenStorage {
	return &InMemoryTokenStorage{token: token}
}

// Store caches the given token.
func (s *InMemoryTokenStorage) Store(token *schemas.AccessToken) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = token
}

// Get returns the cached token, or nil when none is stored.
func (s *InMemoryTokenStorage) Get() *schemas.AccessToken {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token
}

// Clear discards the cached token.
func (s *InMemoryTokenStorage) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = nil
}
