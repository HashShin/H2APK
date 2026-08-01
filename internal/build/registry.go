package build

import (
	"sync"

	"h2apk/internal/types"
)

// Record tracks the state of a single APK build.
type Record struct {
	Status    string
	APKName   string
	Artifacts []types.Artifact
	Err       string
	Log       string
	LogCh     chan string
}

// Registry is a concurrency-safe store of in-progress and completed builds.
type Registry struct {
	mu     sync.RWMutex
	builds map[string]*Record
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{builds: make(map[string]*Record)}
}

// Create initialises a new build record under the given ID.
func (r *Registry) Create(id string) *Record {
	rec := &Record{Status: "building", LogCh: make(chan string, 50)}
	r.mu.Lock()
	r.builds[id] = rec
	r.mu.Unlock()
	return rec
}

// Get retrieves an existing record by ID.
func (r *Registry) Get(id string) (*Record, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.builds[id]
	return rec, ok
}
