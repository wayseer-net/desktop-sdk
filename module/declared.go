package module

import (
	"slices"

	"wayseer.dev/sdk/manifest"
	"wayseer.dev/sdk/model"
)

// Declarer is a module whose signed manifest bounds the actions it may offer (ADR-0010 D7, D8):
// the host offers and runs only those, on only the kinds declared for each.
type Declarer interface {
	// DeclaredActions is what the manifest declares, and false while it isn't known.
	DeclaredActions() ([]Action, bool)
}

// DeclaredIn is the actions man declares, as a catalogue holds them.
func DeclaredIn(man manifest.Manifest) []Action {
	acts := make([]Action, len(man.Actions))
	for i, a := range man.Actions {
		acts[i] = Action{ID: a.ID, Title: a.Title, Changes: a.Changes, Kinds: make([]model.Kind, len(a.Kinds))}
		for j, k := range a.Kinds {
			acts[i].Kinds[j] = model.Kind(k)
		}
	}
	return acts
}

// KeepDeclared is cat cut to the actions declared, each on only its declared kinds, and what
// it leaves out: an action ID, or "ID on kind".
func KeepDeclared(cat, declared []Action) (kept []Action, left []string) {
	for _, a := range cat {
		i := slices.IndexFunc(declared, func(d Action) bool { return d.ID == a.ID })
		if i < 0 {
			left = append(left, a.ID)
			continue
		}
		var kinds []model.Kind
		for _, k := range a.Kinds {
			if slices.Contains(declared[i].Kinds, k) {
				kinds = append(kinds, k)
			} else {
				left = append(left, a.ID+" on "+string(k))
			}
		}
		if len(kinds) > 0 {
			a.Kinds = kinds
			kept = append(kept, a)
		}
	}
	return kept, left
}
