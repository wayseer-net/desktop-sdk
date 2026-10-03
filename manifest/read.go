package manifest

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// read decodes one node at a path such as "kinds[0].kind".
type read func(n *yaml.Node, path string) error

type field struct {
	read     read
	required bool
}

// reader decodes the node tree into a Manifest, noting each field's line for later errors.
type reader struct{ lines map[string]int }

func (r *reader) manifest(n *yaml.Node) (Manifest, error) {
	var m Manifest
	err := r.mapping(n, "", map[string]field{
		"id":             {r.str(&m.ID), true},
		"name":           {r.str(&m.Name), true},
		"description":    {r.str(&m.Description), false},
		"homepage":       {r.str(&m.Homepage), false},
		"version":        {r.str(&m.Version), true},
		"contract":       {r.integer(&m.Contract), true},
		"namespace":      {r.str(&m.Namespace), true},
		"os":             {r.str(&m.OS), true},
		"arch":           {r.str(&m.Arch), true},
		"sha256":         {r.str(&m.SHA256), true},
		"source":         {r.source(&m.Source), false},
		"publisher_cert": {r.str(&m.PublisherCert), false},
		"kinds":          {r.list(maxKinds, func(n *yaml.Node, p string) error { return r.kind(n, p, &m.Kinds) }), false},
		"actions":        {r.list(maxActions, func(n *yaml.Node, p string) error { return r.action(n, p, &m.Actions) }), false},
		"network":        {r.list(maxNetwork, r.appendStr(&m.Network)), false},
		"secrets":        {r.boolean(&m.Secrets), false},
	})
	return m, err
}

func (r *reader) source(dst **Source) read {
	return func(n *yaml.Node, path string) error {
		s := &Source{}
		*dst = s
		return r.mapping(n, path, map[string]field{
			"url":     {r.str(&s.URL), true},
			"tag":     {r.str(&s.Tag), true},
			"commit":  {r.str(&s.Commit), true},
			"licence": {r.str(&s.Licence), true},
		})
	}
}

func (r *reader) kind(n *yaml.Node, path string, dst *[]Kind) error {
	var k Kind
	err := r.mapping(n, path, map[string]field{
		"kind":   {r.str(&k.Kind), true},
		"label":  {r.str(&k.Label), false},
		"plural": {r.str(&k.Plural), false},
		"like":   {r.str(&k.Like), false},
	})
	*dst = append(*dst, k)
	return err
}

func (r *reader) action(n *yaml.Node, path string, dst *[]Action) error {
	var a Action
	err := r.mapping(n, path, map[string]field{
		"id":      {r.str(&a.ID), true},
		"title":   {r.str(&a.Title), true},
		"changes": {r.str(&a.Changes), true},
		"kinds":   {r.list(maxKinds, r.appendStr(&a.Kinds)), true},
	})
	*dst = append(*dst, a)
	return err
}

// mapping reads a mapping's keys into fields, refusing unknown, repeated and missing keys.
func (r *reader) mapping(n *yaml.Node, path string, fields map[string]field) error {
	if n.Kind != yaml.MappingNode {
		return fieldError(n.Line, orTop(path), ErrType)
	}
	seen := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if k.Kind != yaml.ScalarNode {
			return fieldError(k.Line, orTop(path), ErrType)
		}
		at := join(path, k.Value)
		f, ok := fields[k.Value]
		switch {
		case !ok:
			return fieldError(k.Line, join(path, shownKey(k.Value)), ErrUnknownKey)
		case seen[k.Value]:
			return fieldError(k.Line, at, ErrDuplicateKey)
		}
		seen[k.Value] = true
		r.lines[at] = v.Line
		if err := f.read(v, at); err != nil {
			return err
		}
	}
	for _, name := range slices.Sorted(maps.Keys(fields)) {
		if fields[name].required && !seen[name] {
			return fieldError(n.Line, join(path, name), ErrMissing)
		}
	}
	return nil
}

// list reads a sequence of at most limit items; an empty one leaves nothing behind.
func (r *reader) list(limit int, item read) read {
	return func(n *yaml.Node, path string) error {
		switch {
		case n.Kind != yaml.SequenceNode:
			return fieldError(n.Line, path, ErrType)
		case len(n.Content) > limit:
			return fieldError(n.Line, path, ErrBounds)
		}
		for i, c := range n.Content {
			at := fmt.Sprintf("%s[%d]", path, i)
			r.lines[at] = c.Line
			if err := item(c, at); err != nil {
				return err
			}
		}
		return nil
	}
}

// str reads a scalar as text; a YAML null reads as empty.
func (r *reader) str(dst *string) read {
	return func(n *yaml.Node, path string) error {
		if n.Kind != yaml.ScalarNode {
			return fieldError(n.Line, path, ErrType)
		}
		*dst = n.Value
		if n.Tag == "!!null" {
			*dst = ""
		}
		return nil
	}
}

func (r *reader) appendStr(dst *[]string) read {
	return func(n *yaml.Node, path string) error {
		var s string
		if err := r.str(&s)(n, path); err != nil {
			return err
		}
		*dst = append(*dst, s)
		return nil
	}
}

// integer reads a plain, unsigned decimal; anything else (hex, signs, quotes) is the wrong type.
func (r *reader) integer(dst *int) read {
	return func(n *yaml.Node, path string) error {
		if n.Kind != yaml.ScalarNode || n.Style != 0 || !decimal(n.Value) {
			return fieldError(n.Line, path, ErrType)
		}
		v, err := strconv.Atoi(n.Value)
		if err != nil || len(n.Value) > 1 && n.Value[0] == '0' {
			return fieldError(n.Line, path, ErrBounds)
		}
		*dst = v
		return nil
	}
}

// boolean reads a plain true or false; YAML's other spellings are the wrong type.
func (r *reader) boolean(dst *bool) read {
	return func(n *yaml.Node, path string) error {
		if n.Kind != yaml.ScalarNode || n.Style != 0 || n.Value != "true" && n.Value != "false" {
			return fieldError(n.Line, path, ErrType)
		}
		*dst = n.Value == "true"
		return nil
	}
}

func decimal(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// shownKey is an unknown key as an error may show it: a short plain name, else a placeholder.
func shownKey(k string) string {
	if k == "" || len(k) > 32 || strings.ContainsFunc(k, func(c rune) bool { return (c < 'a' || c > 'z') && c != '_' }) {
		return "(a key)"
	}
	return k
}

func orTop(path string) string {
	if path == "" {
		return "the manifest"
	}
	return path
}
