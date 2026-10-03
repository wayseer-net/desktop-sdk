package model

import (
	"maps"
	"slices"
)

func entityEqual(a, b *Entity) bool {
	return a.Ref == b.Ref && a.Kind == b.Kind && a.Name == b.Name && a.Status == b.Status && a.Place == b.Place &&
		a.Source == b.Source && a.Seen.Equal(b.Seen) && slices.Equal(a.Tags, b.Tags) && attrsEqual(a.Attrs, b.Attrs)
}

func edgeEqual(a, b *Edge) bool {
	return a.Key() == b.Key() && a.Weight == b.Weight && a.Traffic == b.Traffic && a.Source == b.Source && attrsEqual(a.Attrs, b.Attrs)
}

func attrsEqual(a, b map[string]Value) bool { return maps.EqualFunc(a, b, Value.Equal) }
