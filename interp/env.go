package interp

import "sync"

// Env is one lexical scope. Tasks spawned by map_par share their enclosing
// scopes, so every scope is guarded by a lock.
type Env struct {
	mu     sync.RWMutex
	vars   map[string]*binding
	parent *Env
}

type binding struct {
	val     Value
	mutable bool
}

func NewEnv(parent *Env) *Env { return &Env{vars: map[string]*binding{}, parent: parent} }

// Define binds name in this scope. A second `let` of the same name replaces
// the first: shadowing.
func (e *Env) Define(name string, v Value, mutable bool) {
	e.mu.Lock()
	e.vars[name] = &binding{val: v, mutable: mutable}
	e.mu.Unlock()
}

func (e *Env) Lookup(name string) (Value, bool) {
	for s := e; s != nil; s = s.parent {
		s.mu.RLock()
		b, ok := s.vars[name]
		var v Value
		if ok {
			v = b.val
		}
		s.mu.RUnlock()
		if ok {
			return v, true
		}
	}
	return nil, false
}

// Assign changes an existing `var`. The checker has already rejected
// assignments to names that are missing or immutable.
func (e *Env) Assign(name string, v Value) bool {
	for s := e; s != nil; s = s.parent {
		s.mu.Lock()
		b, ok := s.vars[name]
		if ok {
			b.val = v
		}
		s.mu.Unlock()
		if ok {
			return true
		}
	}
	return false
}
