package module

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"wayseer/pkg/sdk/model"

	"go.yaml.in/yaml/v3"
)

// Config is one module instance's configuration: its name and its kind-specific options.
type Config struct {
	Name    model.ModuleID
	Line    int       // line of the module entry in the config file
	Options yaml.Node // zero when the entry has no options
}

// Decode decodes the options into v, a pointer to a struct, rejecting unknown options by line.
// Fields keep their values when their options are absent, so set defaults before calling.
func (c Config) Decode(v any) error {
	n := &c.Options
	if n.Kind == 0 || n.Tag == "!!null" {
		return nil
	}
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: options must be a mapping", n.Line)
	}
	if err := checkKnown(n, reflect.TypeOf(v), ""); err != nil {
		return err
	}
	return n.Decode(v)
}

// checkKnown reports the first mapping key under n that no field of t accepts, since
// yaml.Node.Decode ignores unknown fields.
func checkKnown(n *yaml.Node, t reflect.Type, path string) error {
	for n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if opaque(t) {
		return nil
	}
	switch {
	case t.Kind() == reflect.Struct && n.Kind == yaml.MappingNode:
		return checkStruct(n, t, path)
	case t.Kind() == reflect.Map && n.Kind == yaml.MappingNode:
		for i := 1; i < len(n.Content); i += 2 {
			if err := checkKnown(n.Content[i], t.Elem(), path+"."+n.Content[i-1].Value); err != nil {
				return err
			}
		}
	case (t.Kind() == reflect.Slice || t.Kind() == reflect.Array) && n.Kind == yaml.SequenceNode:
		for i, item := range n.Content {
			if err := checkKnown(item, t.Elem(), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}
	return nil // shape mismatches are left for Decode to report
}

// opaque types decode their own YAML: a yaml.Node, or a type with UnmarshalYAML.
func opaque(t reflect.Type) bool {
	return t == yamlNode || reflect.PointerTo(t).Implements(unmarshaler)
}

func checkStruct(n *yaml.Node, t reflect.Type, path string) error {
	fields := yamlFields(t)
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		name := strings.TrimPrefix(path+"."+k.Value, ".")
		ft, ok := fields[k.Value]
		if !ok {
			return fmt.Errorf("line %d: unknown option %q (want %s)", k.Line, name, strings.Join(sortedKeys(fields), ", "))
		}
		if err := checkKnown(v, ft, name); err != nil {
			return err
		}
	}
	return nil
}

// yamlFields maps each option name t accepts to its field type, following yaml v3's tag rules.
func yamlFields(t reflect.Type) map[string]reflect.Type {
	out := map[string]reflect.Type{}
	for f := range t.Fields() {
		tag := f.Tag.Get("yaml")
		name, flags, _ := strings.Cut(tag, ",")
		switch {
		case name == "-" || !f.IsExported() && !f.Anonymous:
		case strings.Contains(flags, "inline"):
			maps.Copy(out, yamlFields(f.Type))
		case name == "":
			out[strings.ToLower(f.Name)] = f.Type
		default:
			out[name] = f.Type
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string { return slices.Sorted(maps.Keys(m)) }

var (
	unmarshaler = reflect.TypeFor[yaml.Unmarshaler]()
	yamlNode    = reflect.TypeFor[yaml.Node]()
)
