// Package worldfile records a world's entities and edges as JSON lines, so a module's world
// can be recorded once and laid out or drawn by tests that do not run the module.
package worldfile

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"

	"wayseer.dev/sdk/model"
)

// worldLine is one entity, or with From set one edge, of a recorded world. Only string
// attributes are kept: what layouts group by, and what labels show.
type worldLine struct {
	Ref    model.EntityRef   `json:"ref,omitempty"`
	Name   string            `json:"name,omitempty"`
	Status string            `json:"status,omitempty"`
	Reason string            `json:"reason,omitempty"`
	Attrs  map[string]string `json:"attrs,omitempty"`
	From   model.EntityRef   `json:"from,omitempty"`
	Rel    model.Relation    `json:"rel,omitempty"`
	To     model.EntityRef   `json:"to,omitempty"`
}

// Record writes cs's entities, then its edges, one JSON object a line, sorted.
func Record(cs *model.ChangeSet) string {
	var lines []worldLine
	for _, e := range cs.Upserts {
		l := worldLine{Ref: e.Ref, Name: e.Name, Status: e.Status.Level.String(), Reason: e.Status.Reason, Attrs: map[string]string{}}
		for k, v := range e.Attrs {
			if v.Type() == model.TypeString {
				l.Attrs[k] = v.Str()
			}
		}
		lines = append(lines, l)
	}
	slices.SortFunc(lines, func(a, b worldLine) int { return strings.Compare(string(a.Ref), string(b.Ref)) })
	n := len(lines)
	for _, e := range cs.Edges {
		lines = append(lines, worldLine{From: e.From, Rel: e.Rel, To: e.To})
	}
	slices.SortFunc(lines[n:], func(a, b worldLine) int {
		return strings.Compare(string(a.From)+" "+string(a.Rel)+" "+string(a.To), string(b.From)+" "+string(b.Rel)+" "+string(b.To))
	})
	var b bytes.Buffer
	for _, l := range lines {
		raw, _ := json.Marshal(l) // strings and maps of strings always marshal
		b.Write(raw)
		b.WriteByte('\n')
	}
	return b.String()
}

var levels = map[string]model.StatusLevel{}

func init() {
	for l := model.StatusUnknown; l <= model.StatusDown; l++ {
		levels[l.String()] = l
	}
}

// ReadFile reads a recorded world from path, gunzipping it when the name ends in .gz.
func ReadFile(path string) (model.ChangeSet, error) {
	raw, err := os.ReadFile(path)
	if err == nil && strings.HasSuffix(path, ".gz") {
		var z *gzip.Reader
		if z, err = gzip.NewReader(bytes.NewReader(raw)); err == nil {
			raw, err = io.ReadAll(z)
		}
	}
	if err != nil {
		return model.ChangeSet{}, fmt.Errorf("%s: %w", path, err)
	}
	return Parse(raw)
}

// Parse reads a world Record wrote.
func Parse(raw []byte) (model.ChangeSet, error) {
	var cs model.ChangeSet
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(nil, 1<<20)
	for n := 1; sc.Scan(); n++ {
		var l worldLine
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			return cs, fmt.Errorf("line %d: %w", n, err)
		}
		if l.From != "" {
			cs.Edges = append(cs.Edges, model.Edge{From: l.From, To: l.To, Rel: l.Rel, Weight: 1, Source: model.ModuleID(l.From.Instance())})
			continue
		}
		level, ok := levels[l.Status]
		if !ok {
			return cs, fmt.Errorf("line %d: unknown status %q", n, l.Status)
		}
		attrs := make(map[string]model.Value, len(l.Attrs))
		for _, k := range slices.Sorted(maps.Keys(l.Attrs)) {
			attrs[k] = model.String(l.Attrs[k])
		}
		cs.Upserts = append(cs.Upserts, model.Entity{
			Ref: l.Ref, Kind: l.Ref.Kind(), Name: l.Name, Status: model.Status{Level: level, Reason: l.Reason},
			Attrs: attrs, Source: model.ModuleID(l.Ref.Instance()),
		})
	}
	return cs, sc.Err()
}
