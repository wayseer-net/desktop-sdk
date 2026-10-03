package manifest

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxName        = 64
	maxDescription = 512
	maxURL         = 512
	maxContract    = 1000
	maxVersion     = 64
	maxPlatform    = 16
	maxCertID      = 64
	maxTag         = 128
	maxLicence     = 64
	maxLabel       = 64
	maxTitle       = 80
	maxChanges     = 200
	maxEndpoint    = 256
	maxKinds       = 64
	maxActions     = 64
	maxNetwork     = 32
)

// at makes a field's error, with its line when the manifest was parsed.
type at func(path string, err error) error

// validate checks every field's bounds and how kinds and actions refer to each other.
func (m Manifest) validate(lines map[string]int) error {
	fail := at(func(path string, err error) error { return fieldError(lines[path], path, err) })
	for _, f := range []struct {
		path string
		ok   bool
	}{
		{"id", validID(m.ID)},
		{"name", text(m.Name, maxName)},
		{"description", m.Description == "" || text(m.Description, maxDescription)},
		{"homepage", m.Homepage == "" || httpsURL(m.Homepage)},
		{"version", validVersion(m.Version)},
		{"contract", m.Contract >= 1 && m.Contract <= maxContract},
		{"namespace", ValidNamespace(m.Namespace)},
		{"os", platform(m.OS)},
		{"arch", platform(m.Arch)},
		{"sha256", lowerHex(m.SHA256, 64)},
		{"publisher_cert", m.PublisherCert == "" || certID(m.PublisherCert)},
		{"kinds", len(m.Kinds) <= maxKinds},
		{"actions", len(m.Actions) <= maxActions},
		{"network", len(m.Network) <= maxNetwork},
	} {
		if !f.ok {
			return fail(f.path, ErrBounds)
		}
	}
	if Reserved(m.Namespace) {
		return fail("namespace", ErrReserved)
	}
	for _, check := range []func(at) error{m.checkSource, m.checkNetwork, m.checkKinds, m.checkActions} {
		if err := check(fail); err != nil {
			return err
		}
	}
	return nil
}

func (m Manifest) checkSource(fail at) error {
	s := m.Source
	if s == nil {
		return nil
	}
	for _, f := range []struct {
		path string
		ok   bool
	}{
		{"source.url", httpsURL(s.URL)},
		{"source.tag", gitTag(s.Tag)},
		{"source.commit", lowerHex(s.Commit, 40) || lowerHex(s.Commit, 64)},
		{"source.licence", spdx(s.Licence)},
	} {
		if !f.ok {
			return fail(f.path, ErrBounds)
		}
	}
	return nil
}

func (m Manifest) checkNetwork(fail at) error {
	for i, e := range m.Network {
		if !text(e, maxEndpoint) {
			return fail(fmt.Sprintf("network[%d]", i), ErrBounds)
		}
	}
	return nil
}

func (m Manifest) checkKinds(fail at) error {
	seen := map[string]bool{}
	for i, k := range m.Kinds {
		p := fmt.Sprintf("kinds[%d]", i)
		switch {
		case !IsCoreKind(k.Kind) && !inNamespace(k.Kind, m.Namespace):
			return fail(p+".kind", ErrKind)
		case seen[k.Kind]:
			return fail(p+".kind", ErrRepeated)
		case k.Like != "" && !IsCoreKind(k.Like):
			return fail(p+".like", ErrKind)
		case k.Label != "" && !text(k.Label, maxLabel):
			return fail(p+".label", ErrBounds)
		case k.Plural != "" && !text(k.Plural, maxLabel):
			return fail(p+".plural", ErrBounds)
		}
		seen[k.Kind] = true
	}
	return nil
}

func (m Manifest) checkActions(fail at) error {
	seen := map[string]bool{}
	for i, a := range m.Actions {
		p := fmt.Sprintf("actions[%d]", i)
		switch {
		case !ValidActionID(a.ID):
			return fail(p+".id", ErrBounds)
		case seen[a.ID]:
			return fail(p+".id", ErrRepeated)
		case !text(a.Title, maxTitle):
			return fail(p+".title", ErrBounds)
		case !text(a.Changes, maxChanges):
			return fail(p+".changes", ErrBounds)
		case len(a.Kinds) == 0:
			return fail(p+".kinds", ErrMissing)
		case len(a.Kinds) > maxKinds:
			return fail(p+".kinds", ErrBounds)
		}
		seen[a.ID] = true
		if err := m.checkActionKinds(fail, p, a.Kinds); err != nil {
			return err
		}
	}
	return nil
}

// checkActionKinds checks an action applies only to kinds the manifest lists, each once.
func (m Manifest) checkActionKinds(fail at, path string, kinds []string) error {
	seen := map[string]bool{}
	for j, k := range kinds {
		p := fmt.Sprintf("%s.kinds[%d]", path, j)
		switch {
		case !m.lists(k):
			return fail(p, ErrKind)
		case seen[k]:
			return fail(p, ErrRepeated)
		}
		seen[k] = true
	}
	return nil
}

func (m Manifest) lists(kind string) bool {
	for _, k := range m.Kinds {
		if k.Kind == kind {
			return true
		}
	}
	return false
}

// validID checks "<publisher>/<name>", each part with a namespace's shape.
func validID(id string) bool {
	pub, name, ok := strings.Cut(id, "/")
	return ok && ValidNamespace(pub) && ValidNamespace(name)
}

// text checks one line of printable text with no space at either end.
func text(s string, limit int) bool {
	return s != "" && len(s) <= limit && utf8.ValidString(s) && s == strings.TrimSpace(s) &&
		!strings.ContainsFunc(s, func(c rune) bool { return !unicode.IsPrint(c) })
}

func httpsURL(s string) bool {
	if !text(s, maxURL) || strings.ContainsRune(s, ' ') {
		return false
	}
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil
}

func platform(s string) bool {
	return s != "" && len(s) <= maxPlatform && lower(rune(s[0])) &&
		!strings.ContainsFunc(s, func(c rune) bool { return !lower(c) && !digit(c) })
}

func lowerHex(s string, n int) bool {
	return len(s) == n && !strings.ContainsFunc(s, func(c rune) bool { return !digit(c) && (c < 'a' || c > 'f') })
}

func certID(s string) bool {
	return s != "" && len(s) <= maxCertID && !strings.ContainsFunc(s, func(c rune) bool {
		return !lower(c) && !digit(c) && (c < 'A' || c > 'Z') && !strings.ContainsRune("._-", c)
	})
}

func gitTag(s string) bool {
	return s != "" && len(s) <= maxTag && !strings.ContainsFunc(s, func(c rune) bool {
		return !lower(c) && !digit(c) && (c < 'A' || c > 'Z') && !strings.ContainsRune("._+/-", c)
	})
}

// spdx checks an SPDX identifier or expression's characters, such as "MIT OR Apache-2.0".
func spdx(s string) bool {
	return text(s, maxLicence) && !strings.ContainsFunc(s, func(c rune) bool {
		return !lower(c) && !digit(c) && (c < 'A' || c > 'Z') && !strings.ContainsRune(".+-() ", c)
	})
}
