package session

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"
)

// ErrNoBackendRoute reports attempts to read/write paths without a registered backend.
var ErrNoBackendRoute = errors.New("session: no backend route for path")

// Backend defines the minimal persistence operations required by sessions.
type Backend interface {
	Read(path string) ([]byte, error)
	Write(path string, data []byte) error
	List(prefix string) ([]string, error)
	Delete(path string) error
}

// CompositeBackend routes operations to backends based on the longest matching prefix.
type CompositeBackend struct {
	mu     sync.RWMutex
	routes map[string]Backend
}

// NewCompositeBackend creates an empty CompositeBackend.
func NewCompositeBackend() *CompositeBackend {
	return &CompositeBackend{
		routes: make(map[string]Backend),
	}
}

// AddRoute registers a backend for the specified prefix, overwriting existing entries.
func (b *CompositeBackend) AddRoute(prefix string, backend Backend) error {
	if backend == nil {
		return errors.New("session: backend cannot be nil")
	}
	norm := normalizePrefix(prefix)
	if norm == "" {
		return errors.New("session: prefix cannot be empty")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.routes[norm] = backend
	return nil
}

// Route returns the backend registered for the provided path (longest prefix match).
func (b *CompositeBackend) Route(p string) Backend {
	if b == nil {
		return nil
	}
	path := normalizePath(p)

	b.mu.RLock()
	defer b.mu.RUnlock()

	var matched Backend
	var maxLen int
	for prefix, backend := range b.routes {
		if strings.HasPrefix(path, prefix) && len(prefix) > maxLen {
			matched = backend
			maxLen = len(prefix)
		}
	}
	return matched
}

// Read resolves the backend for path and delegates the read.
func (b *CompositeBackend) Read(path string) ([]byte, error) {
	backend, err := b.routeOrErr(path)
	if err != nil {
		return nil, err
	}
	return backend.Read(normalizePath(path))
}

// Write resolves the backend for path and delegates the write.
func (b *CompositeBackend) Write(path string, data []byte) error {
	backend, err := b.routeOrErr(path)
	if err != nil {
		return err
	}
	return backend.Write(normalizePath(path), data)
}

// List resolves the backend for prefix and delegates the list operation.
func (b *CompositeBackend) List(prefix string) ([]string, error) {
	backend, err := b.routeOrErr(prefix)
	if err != nil {
		return nil, err
	}
	return backend.List(normalizePath(prefix))
}

// Delete resolves the backend for path and delegates the delete.
func (b *CompositeBackend) Delete(path string) error {
	backend, err := b.routeOrErr(path)
	if err != nil {
		return err
	}
	return backend.Delete(normalizePath(path))
}

func (b *CompositeBackend) routeOrErr(path string) (Backend, error) {
	backend := b.Route(path)
	if backend == nil {
		return nil, fmt.Errorf("%w: %s", ErrNoBackendRoute, normalizePath(path))
	}
	return backend, nil
}

func normalizePrefix(prefix string) string {
	p := normalizePath(prefix)
	if p == "/" {
		return p
	}
	if !strings.HasSuffix(p, "/") {
		return p
	}
	// normalizePath already removes trailing slashes; just return.
	return p
}

func normalizePath(p string) string {
	if p == "" {
		return "/"
	}
	clean := path.Clean(p)
	if clean == "." {
		return "/"
	}
	if !strings.HasPrefix(clean, "/") {
		clean = "/" + clean
	}
	return clean
}
