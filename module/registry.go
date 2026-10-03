package module

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"wayseer.dev/sdk/model"
)

// Factory makes an unconfigured module.
type Factory func() Module

// Registry maps module kinds to factories; modules register in init.
type Registry struct {
	mu    sync.Mutex
	kinds map[string]Factory
}

// Default is the registry first-party modules add themselves to.
var Default Registry

// Register adds kind to the Default registry; license.Paid says whether a key must unlock it.
func Register(kind string, f Factory) { Default.Register(kind, f) }

// Register adds kind; it panics on a malformed or duplicate kind, which is a programming error.
func (r *Registry) Register(kind string, f Factory) {
	if err := model.ModuleID(kind).Validate(); err != nil {
		panic(fmt.Sprintf("module kind %q: %v", kind, err))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.kinds[kind]; dup {
		panic(fmt.Sprintf("module kind %q registered twice", kind))
	}
	if r.kinds == nil {
		r.kinds = map[string]Factory{}
	}
	r.kinds[kind] = f
}

// Kinds lists the registered kinds in order.
func (r *Registry) Kinds() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return sortedKeys(r.kinds)
}

func (r *Registry) factory(kind string) (Factory, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, ok := r.kinds[kind]
	return f, ok
}

func (r *Registry) known() string {
	if kinds := r.Kinds(); len(kinds) > 0 {
		return "known: " + strings.Join(kinds, ", ")
	}
	return "no module kinds are built in"
}

// New makes a module of kind and configures it with c; an unknown kind's error lists the known.
func (r *Registry) New(ctx context.Context, kind string, c Config) (Module, error) {
	f, ok := r.factory(kind)
	if !ok {
		return nil, fmt.Errorf("unknown module kind %q (%s)", kind, r.known())
	}
	if err := c.Name.Validate(); err != nil {
		return nil, err
	}
	m := f()
	if err := m.Configure(ctx, c); err != nil {
		return nil, err
	}
	return m, nil
}
