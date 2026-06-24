package review

import (
	"fmt"
	"sort"
	"sync"
)

// Registry holds the registered reviewable datasets. Datasets register at server
// construction; the platform (endpoints, sampler) looks them up by id.
type Registry struct {
	mu       sync.RWMutex
	datasets map[string]ReviewableDataset
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{datasets: make(map[string]ReviewableDataset)}
}

// Register adds a dataset. Returns an error on empty or duplicate id.
func (r *Registry) Register(d ReviewableDataset) error {
	if d == nil {
		return fmt.Errorf("review: cannot register nil dataset")
	}
	id := d.ID()
	if id == "" {
		return fmt.Errorf("review: dataset has empty ID")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.datasets[id]; exists {
		return fmt.Errorf("review: dataset %q already registered", id)
	}
	r.datasets[id] = d
	return nil
}

// Get returns the dataset with the given id.
func (r *Registry) Get(id string) (ReviewableDataset, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.datasets[id]
	return d, ok
}

// List returns all registered datasets, sorted by id for stable output.
func (r *Registry) List() []ReviewableDataset {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ReviewableDataset, 0, len(r.datasets))
	for _, d := range r.datasets {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}
