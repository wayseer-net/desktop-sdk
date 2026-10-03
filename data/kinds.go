package data

import (
	"errors"
	"fmt"
	"sync"
	"wayseer/pkg/sdk/model"
)

// ErrForeignKind reports an entity whose kind is neither a core kind nor in its module's namespace.
var ErrForeignKind = errors.New("kind outside the module's namespace")

// ForeignKindError names the first foreign kind in a refused change set.
type ForeignKindError struct {
	Source model.ModuleID
	Kind   model.Kind
}

func (e *ForeignKindError) Error() string {
	return fmt.Sprintf("%s: %s sends kind %s", ErrForeignKind, e.Source, e.Kind)
}

func (e *ForeignKindError) Unwrap() error { return ErrForeignKind }

// confined holds each confined source's namespace; a source not in it may send any kind.
type confined struct{ ns sync.Map } // model.ModuleID → string

func (c *confined) set(src model.ModuleID, ns string) { c.ns.Store(src, ns) }

func (c *confined) free(src model.ModuleID) { c.ns.Delete(src) }

// check refuses cs if src is confined and an upsert's kind is neither core nor in its namespace.
func (c *confined) check(src model.ModuleID, cs *model.ChangeSet) error {
	v, ok := c.ns.Load(src)
	if !ok {
		return nil
	}
	ns := v.(string)
	for i := range cs.Upserts {
		k := cs.Upserts[i].Kind
		if !k.IsCore() && (ns == "" || k.Namespace() != ns) {
			return &ForeignKindError{Source: src, Kind: k}
		}
	}
	return nil
}

// Confine limits src's entities to core kinds and those in namespace ns, until RemoveSource;
// with ns "", to core kinds.
func (c *Coalescer) Confine(src model.ModuleID, ns string) { c.kinds.set(src, ns) }
