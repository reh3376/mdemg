package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// State represents the circuit breaker state.
type State int

const (
	// StateClosed means the circuit is functioning normally.
	StateClosed State = iota
	// StateOpen means the circuit is open and requests are rejected.
	StateOpen
	// StateHalfOpen means the circuit is testing if the service has recovered.
	StateHalfOpen
)

// String returns the state name.
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// ErrCircuitOpen is returned when the circuit is open.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// Breaker implements the circuit breaker pattern.
// It is safe for concurrent use.
type Breaker struct {
	name   string
	cfg    Config
	mu     sync.RWMutex
	state  State
	counts counts

	// openedAt is when the circuit was opened
	openedAt time.Time

	// halfOpenRequests tracks concurrent requests in half-open state
	halfOpenRequests int32

	// onStateChange is called when the state changes
	onStateChange func(name string, from, to State)
}

// counts tracks success/failure counts.
type counts struct {
	failures  int
	successes int
	total     int64
}

// New creates a new circuit breaker with the given name and config.
func New(name string, cfg Config) *Breaker {
	return &Breaker{
		name:  name,
		cfg:   cfg,
		state: StateClosed,
	}
}

// OnStateChange sets a callback for state changes.
func (b *Breaker) OnStateChange(fn func(name string, from, to State)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.onStateChange = fn
}

// Name returns the circuit breaker name.
func (b *Breaker) Name() string {
	return b.name
}

// State returns the current state.
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.currentState()
}

// currentState returns the state, checking for timeout transitions.
// Must be called with b.mu held (write lock needed for state transitions).
func (b *Breaker) currentState() State {
	if b.state == StateOpen {
		if time.Since(b.openedAt) >= b.cfg.Timeout {
			// Actually transition to half-open
			b.state = StateHalfOpen
			b.counts.failures = 0
			b.counts.successes = 0
			atomic.StoreInt32(&b.halfOpenRequests, 0)
			if b.onStateChange != nil {
				go b.onStateChange(b.name, StateOpen, StateHalfOpen)
			}
		}
	}
	return b.state
}

// Allow checks if a request should be allowed.
// Returns true if the request can proceed, false if the circuit is open.
func (b *Breaker) Allow() bool {
	if !b.cfg.Enabled {
		return true
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	state := b.currentState()

	switch state {
	case StateClosed:
		return true

	case StateOpen:
		return false

	case StateHalfOpen:
		// Limit concurrent requests in half-open state
		if b.cfg.MaxConcurrent > 0 {
			current := atomic.LoadInt32(&b.halfOpenRequests)
			if int(current) >= b.cfg.MaxConcurrent {
				return false
			}
			atomic.AddInt32(&b.halfOpenRequests, 1)
		}
		return true

	default:
		return true
	}
}

// Execute runs the given function with circuit breaker protection.
// If the circuit is open, returns ErrCircuitOpen immediately.
func (b *Breaker) Execute(ctx context.Context, fn func(context.Context) error) error {
	if !b.cfg.Enabled {
		return fn(ctx)
	}

	if !b.Allow() {
		return ErrCircuitOpen
	}

	// Track half-open request completion
	state := b.State()
	defer func() {
		if state == StateHalfOpen && b.cfg.MaxConcurrent > 0 {
			atomic.AddInt32(&b.halfOpenRequests, -1)
		}
	}()

	err := fn(ctx)

	if err != nil {
		b.RecordFailure()
	} else {
		b.RecordSuccess()
	}

	return err
}

// RecordSuccess records a successful request.
func (b *Breaker) RecordSuccess() {
	if !b.cfg.Enabled {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.counts.total++

	switch b.currentState() {
	case StateClosed:
		// Reset failure count on success
		b.counts.failures = 0

	case StateHalfOpen:
		b.counts.successes++
		if b.counts.successes >= b.cfg.SuccessThreshold {
			b.transition(StateClosed)
		}

	case StateOpen:
		// Shouldn't happen, but handle gracefully
	}
}

// RecordFailure records a failed request.
func (b *Breaker) RecordFailure() {
	if !b.cfg.Enabled {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.counts.total++
	b.counts.failures++

	switch b.currentState() {
	case StateClosed:
		if b.counts.failures >= b.cfg.FailureThreshold {
			b.transition(StateOpen)
		}

	case StateHalfOpen:
		// Any failure in half-open reopens the circuit
		b.transition(StateOpen)

	case StateOpen:
		// Already open, nothing to do
	}
}

// transition changes the state and resets counts.
// Must be called with b.mu held.
func (b *Breaker) transition(to State) {
	from := b.state
	if from == to {
		return
	}

	b.state = to
	b.counts.failures = 0
	b.counts.successes = 0
	atomic.StoreInt32(&b.halfOpenRequests, 0)

	if to == StateOpen {
		b.openedAt = time.Now()
	}

	if b.onStateChange != nil {
		// Call outside of lock to prevent deadlocks
		go b.onStateChange(b.name, from, to)
	}
}

// Reset forces the circuit to closed state.
func (b *Breaker) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.transition(StateClosed)
}

// Counts returns the current count statistics.
func (b *Breaker) Counts() (failures, successes int, total int64) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.counts.failures, b.counts.successes, b.counts.total
}

// Registry manages multiple circuit breakers by name.
type Registry struct {
	mu            sync.RWMutex
	breakers      map[string]*Breaker
	cfg           Config
	onStateChange func(name string, from, to State) // propagated to lazily created breakers
}

// NewRegistry creates a registry with the given default config.
func NewRegistry(cfg Config) *Registry {
	return &Registry{
		breakers: make(map[string]*Breaker),
		cfg:      cfg,
	}
}

// SetOnStateChange sets a callback that will be propagated to all existing
// and future breakers created by this registry.
func (r *Registry) SetOnStateChange(fn func(name string, from, to State)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onStateChange = fn
	for _, b := range r.breakers {
		b.OnStateChange(fn)
	}
}

// Get returns the circuit breaker for the given name, creating one if needed.
func (r *Registry) Get(name string) *Breaker {
	r.mu.RLock()
	b, exists := r.breakers[name]
	r.mu.RUnlock()

	if exists {
		return b
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if b, exists = r.breakers[name]; exists {
		return b
	}

	b = New(name, r.cfg)
	if r.onStateChange != nil {
		b.OnStateChange(r.onStateChange)
	}
	r.breakers[name] = b
	return b
}

// All returns all registered breakers.
func (r *Registry) All() map[string]*Breaker {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]*Breaker, len(r.breakers))
	for k, v := range r.breakers {
		result[k] = v
	}
	return result
}

// States returns all breaker states.
func (r *Registry) States() map[string]State {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]State, len(r.breakers))
	for k, v := range r.breakers {
		result[k] = v.State()
	}
	return result
}
