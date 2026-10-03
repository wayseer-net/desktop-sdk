// Package manifest reads and writes a module package's manifest.yaml strictly: only the known
// fields, each within its bounds, with no anchors, aliases, tags or second document.
package manifest

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"go.yaml.in/yaml/v3"
)

// MaxSize is the largest manifest Parse reads, in bytes.
const MaxSize = 64 << 10

// The reasons a manifest is refused. None holds a field's value.
var (
	ErrSize         = errors.New("the manifest is too large")
	ErrSyntax       = errors.New("the manifest isn't valid YAML")
	ErrDocuments    = errors.New("the manifest holds more than one document")
	ErrAnchor       = errors.New("anchors aren't allowed")
	ErrAlias        = errors.New("aliases aren't allowed")
	ErrTag          = errors.New("tags aren't allowed")
	ErrUnknownKey   = errors.New("unknown key")
	ErrDuplicateKey = errors.New("the key is repeated")
	ErrType         = errors.New("the wrong type")
	ErrMissing      = errors.New("missing")
	ErrBounds       = errors.New("outside its bounds")
	ErrReserved     = errors.New("the namespace is reserved for Wayseer")
	ErrKind         = errors.New("the kind is neither a core kind nor in the manifest's namespace")
	ErrRepeated     = errors.New("listed twice")
	ErrMarketplace  = errors.New("a marketplace package needs it")
)

// Manifest describes a module package (module platform plan §4.2). Who signed it comes only
// from the package's signature, never from here.
type Manifest struct {
	ID            string   `yaml:"id"`
	Name          string   `yaml:"name"`
	Description   string   `yaml:"description,omitempty"`
	Homepage      string   `yaml:"homepage,omitempty"`
	Version       string   `yaml:"version"`
	Contract      int      `yaml:"contract"`
	Namespace     string   `yaml:"namespace"`
	OS            string   `yaml:"os"`
	Arch          string   `yaml:"arch"`
	SHA256        string   `yaml:"sha256"`
	Source        *Source  `yaml:"source,omitempty"`
	PublisherCert string   `yaml:"publisher_cert,omitempty"`
	Kinds         []Kind   `yaml:"kinds,omitempty"`
	Actions       []Action `yaml:"actions,omitempty"`
	Network       []string `yaml:"network,omitempty"`
	Secrets       bool     `yaml:"secrets,omitempty"`
}

// Source is where a module's code is published: a repository, a tag, its full commit and the
// code's SPDX licence.
type Source struct {
	URL     string `yaml:"url"`
	Tag     string `yaml:"tag"`
	Commit  string `yaml:"commit"`
	Licence string `yaml:"licence"`
}

// Kind is an entity kind the module emits, with optional display hints.
type Kind struct {
	Kind   string `yaml:"kind"`
	Label  string `yaml:"label,omitempty"`
	Plural string `yaml:"plural,omitempty"`
	Like   string `yaml:"like,omitempty"`
}

// Action is one action the module may offer, as its module.Actor catalogue does.
type Action struct {
	ID      string   `yaml:"id"`
	Title   string   `yaml:"title"`
	Changes string   `yaml:"changes"`
	Kinds   []string `yaml:"kinds"`
}

// Parse reads a manifest, refusing anything outside the format.
func Parse(data []byte) (Manifest, error) {
	m, lines, err := decode(data, false)
	if err != nil {
		return Manifest{}, err
	}
	return m, m.validate(lines)
}

// Platform is what a package's executable fixes in its manifest.
type Platform struct{ OS, Arch, SHA256 string }

// Fill reads a manifest whose platform fields may be absent or stale, sets them to p, and
// returns the manifest's bytes as a package carries them. Errors name the input's lines.
func Fill(data []byte, p Platform) ([]byte, Manifest, error) {
	m, lines, err := decode(data, true)
	if err != nil {
		return nil, Manifest{}, err
	}
	m.OS, m.Arch, m.SHA256 = p.OS, p.Arch, p.SHA256
	if err := m.validate(lines); err != nil {
		return nil, Manifest{}, err
	}
	out, err := Marshal(m)
	return out, m, err
}

// Marshal writes m as YAML that Parse reads back unchanged, refusing an invalid m.
func Marshal(m Manifest) ([]byte, error) {
	if err := m.validate(nil); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return b.Bytes(), enc.Close()
}

// decode reads a manifest's fields without checking their bounds, noting each one's line.
func decode(data []byte, filling bool) (Manifest, map[string]int, error) {
	if len(data) > MaxSize {
		return Manifest{}, nil, ErrSize
	}
	root, err := document(data)
	if err != nil {
		return Manifest{}, nil, err
	}
	r := reader{lines: map[string]int{}, filling: filling}
	m, err := r.manifest(root)
	return m, r.lines, err
}

// Marketplace checks the fields a marketplace package must carry beyond a developer one.
func (m Manifest) Marketplace() error {
	switch {
	case m.Source == nil:
		return fmt.Errorf("manifest: source: %w", ErrMarketplace)
	case m.PublisherCert == "":
		return fmt.Errorf("manifest: publisher_cert: %w", ErrMarketplace)
	}
	return nil
}

// document reads exactly one YAML document and refuses anchors, aliases and tags anywhere in it.
func document(data []byte) (*yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSyntax, err)
	}
	var more yaml.Node
	if err := dec.Decode(&more); !errors.Is(err, io.EOF) {
		return nil, ErrDocuments
	}
	for _, check := range []func(*yaml.Node) error{noAlias, noAnchorOrTag} {
		if err := walk(&doc, check); err != nil {
			return nil, err
		}
	}
	return doc.Content[0], nil
}

func walk(n *yaml.Node, check func(*yaml.Node) error) error {
	if err := check(n); err != nil {
		return err
	}
	for _, c := range n.Content {
		if err := walk(c, check); err != nil {
			return err
		}
	}
	return nil
}

func noAlias(n *yaml.Node) error {
	if n.Kind == yaml.AliasNode {
		return fmt.Errorf("manifest line %d: %w", n.Line, ErrAlias)
	}
	return nil
}

func noAnchorOrTag(n *yaml.Node) error {
	switch {
	case n.Anchor != "":
		return fmt.Errorf("manifest line %d: %w", n.Line, ErrAnchor)
	case n.Style&yaml.TaggedStyle != 0:
		return fmt.Errorf("manifest line %d: %w", n.Line, ErrTag)
	}
	return nil
}

// fieldError names the line, when known, and the field's path, never its value.
func fieldError(line int, path string, err error) error {
	if line == 0 {
		return fmt.Errorf("manifest: %s: %w", path, err)
	}
	return fmt.Errorf("manifest line %d: %s: %w", line, path, err)
}
