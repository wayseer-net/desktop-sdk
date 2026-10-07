package manifest

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func example(t testing.TB) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// edit swaps one exact piece of the example for another, failing if the piece is missing.
func edit(t *testing.T, from, to string) []byte {
	t.Helper()
	b := example(t)
	if !bytes.Contains(b, []byte(from)) {
		t.Fatalf("the example has no %q", from)
	}
	return bytes.Replace(b, []byte(from), []byte(to), 1)
}

func TestParseReadsThePlansExample(t *testing.T) {
	m, err := Parse(example(t))
	if err != nil {
		t.Fatal(err)
	}
	want := Manifest{
		ID: "acme/snowflake", Name: "Snowflake",
		Description: "Warehouses, queries and credit use from Snowflake.",
		Homepage:    "https://github.com/acme/wayseer-snowflake",
		Version:     "1.4.0", Contract: 1, Namespace: "snowflake", OS: "linux", Arch: "amd64",
		SHA256: "9f2c4e8a1b3d5f7092c4e6a8b0d2f4a6c8e0b2d4f6a8c0e2b4d6f8a0c2e4b6d8",
		Source: &Source{
			URL: "https://github.com/acme/wayseer-snowflake", Tag: "v1.4.0",
			Commit: "3b1e5a7c9d2f4b6e8a0c1d3f5b7e9a2c4d6f8b0e", Licence: "Apache-2.0",
		},
		PublisherCert: "dev-7f3a91c2",
		Kinds:         []Kind{{Kind: "snowflake/warehouse", Label: "Warehouse", Like: "database"}},
		Actions: []Action{{
			ID: "resume", Title: "Resume warehouse", Changes: "Starts a suspended warehouse",
			Kinds: []string{"snowflake/warehouse"},
		}},
		Network: []string{"configured by user"},
		Secrets: true,
	}
	if !reflect.DeepEqual(m, want) {
		t.Errorf("parsed\n%+v\nwant\n%+v", m, want)
	}
	if err := m.Marketplace(); err != nil {
		t.Errorf("the example as a marketplace package: %v", err)
	}
}

func TestMarshalRoundTrips(t *testing.T) {
	m, err := Parse(example(t))
	if err != nil {
		t.Fatal(err)
	}
	out, err := Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Parse(out)
	if err != nil || !reflect.DeepEqual(again, m) {
		t.Fatalf("Marshal gave\n%s\nwhich parses as %+v, %v", out, again, err)
	}
	if out2, _ := Marshal(again); !bytes.Equal(out, out2) {
		t.Errorf("Marshal isn't stable:\n%s\nthen\n%s", out, out2)
	}
}

func TestMarshalRefusesAnInvalidManifest(t *testing.T) {
	m, _ := Parse(example(t))
	m.Version = "1.4"
	if _, err := Marshal(m); !errors.Is(err, ErrBounds) {
		t.Errorf("Marshal of a bad version: %v; want %v", err, ErrBounds)
	}
}

func TestParseAcceptsAMinimalDeveloperManifest(t *testing.T) {
	minimal := "id: acme/tool\nname: Tool\nversion: 0.1.0-rc.1+build.5\ncontract: 1\nnamespace: acme\n" +
		"os: linux\narch: arm64\nsha256: " + strings.Repeat("0", 64) + "\n"
	m, err := Parse([]byte(minimal))
	if err != nil {
		t.Fatal(err)
	}
	if m.Source != nil || m.Kinds != nil || m.Actions != nil || m.Network != nil || m.Secrets {
		t.Errorf("optional fields: %+v; want none", m)
	}
	if err := m.Marketplace(); !errors.Is(err, ErrMarketplace) {
		t.Errorf("a manifest without source as a marketplace package: %v; want %v", err, ErrMarketplace)
	}
}

func TestParseRefuses(t *testing.T) {
	big := append(example(t), []byte("# "+strings.Repeat("x", MaxSize))...)
	for _, c := range []struct {
		name string
		in   []byte
		want error
	}{
		{"too large", big, ErrSize},
		{"not YAML", []byte("id: [unclosed\n"), ErrSyntax},
		{"empty", nil, ErrSyntax},
		{"two documents", append(example(t), []byte("---\nid: acme/other\n")...), ErrDocuments},
		{"a list at the top", []byte("- id: acme/snowflake\n"), ErrType},
		{"an unknown key", edit(t, "secrets: true", "secrets: true\nsigner: wayseer"), ErrUnknownKey},
		{"an unknown nested key", edit(t, "licence: Apache-2.0", "licence: Apache-2.0, branch: main"), ErrUnknownKey},
		{"a duplicate key", edit(t, "os: linux", "os: linux\nos: windows"), ErrDuplicateKey},
		{"a merge key", edit(t, "secrets: true", "secrets: true\n<<: {os: windows}"), ErrUnknownKey},
		{"an anchor", edit(t, "kind: snowflake/warehouse,", "kind: &k snowflake/warehouse,"), ErrAnchor},
		{"an alias", bytes.Replace(edit(t, "kind: snowflake/warehouse,", "kind: &k snowflake/warehouse,"),
			[]byte("kinds: [snowflake/warehouse]"), []byte("kinds: [*k]"), 1), ErrAlias},
		{"a tag", edit(t, "contract: 1", "contract: !!int 1"), ErrTag},
		{"a custom tag", edit(t, "name: Snowflake", "name: !x Snowflake"), ErrTag},
		{"a missing field", edit(t, "arch: amd64\n", ""), ErrMissing},
		{"a missing nested field", edit(t, ", tag: v1.4.0", ""), ErrMissing},
		{"a mapping for a string", edit(t, "name: Snowflake", "name: {a: b}"), ErrType},
		{"a string for a number", edit(t, "contract: 1", "contract: one"), ErrType},
		{"a hex contract", edit(t, "contract: 1", "contract: 0x1"), ErrType},
		{"a string for a bool", edit(t, "secrets: true", "secrets: yes"), ErrType},
		{"a capital bool", edit(t, "secrets: true", "secrets: True"), ErrType},
		{"a string for a list", edit(t, "network: [configured by user]", "network: everywhere"), ErrType},
		{"a contract of 0", edit(t, "contract: 1", "contract: 0"), ErrBounds},
		{"a huge contract", edit(t, "contract: 1", "contract: 99999999999999999999"), ErrBounds},
		{"an id with no publisher", edit(t, "id: acme/snowflake", "id: snowflake"), ErrBounds},
		{"an id with a path", edit(t, "id: acme/snowflake", "id: acme/../snowflake"), ErrBounds},
		{"an upper-case id", edit(t, "id: acme/snowflake", "id: Acme/snowflake"), ErrBounds},
		{"a two-part version", edit(t, "version: 1.4.0", "version: \"1.4\""), ErrBounds},
		{"a leading-zero version", edit(t, "version: 1.4.0", "version: 1.04.0"), ErrBounds},
		{"a v version", edit(t, "version: 1.4.0", "version: v1.4.0"), ErrBounds},
		{"an empty name", edit(t, "name: Snowflake", "name: \"\""), ErrBounds},
		{"a control character", edit(t, "name: Snowflake", "name: \"Snow\\u0007flake\""), ErrBounds},
		{"a bidi override", edit(t, "name: Snowflake", "name: \"Snow\\u202eflake\""), ErrBounds},
		{"a two-line description", edit(t, "description: Warehouses, queries and credit use from Snowflake.", `description: "a\nb"`), ErrBounds},
		{"a long name", edit(t, "name: Snowflake", "name: "+strings.Repeat("s", 65)), ErrBounds},
		{"an http homepage", edit(t, "homepage: https://", "homepage: http://"), ErrBounds},
		{"a homepage with a password", edit(t, "homepage: https://", "homepage: https://u:p@"), ErrBounds},
		{"a short hash", edit(t, "sha256: 9f2c", "sha256: 9f2"), ErrBounds},
		{"an upper-case hash", edit(t, "sha256: 9f2c", "sha256: 9F2C"), ErrBounds},
		{"a short commit", edit(t, "commit: 3b1e5a7c9d2f4b6e8a0c1d3f5b7e9a2c4d6f8b0e", "commit: 3b1e5a7"), ErrBounds},
		{"a path os", edit(t, "os: linux", "os: ../linux"), ErrBounds},
		{"a bad namespace", edit(t, "namespace: snowflake", "namespace: Snow_flake"), ErrBounds},
		{"a reserved namespace", edit(t, "namespace: snowflake", "namespace: k8s"), ErrReserved},
		{"a bad publisher cert", edit(t, "publisher_cert: dev-7f3a91c2", "publisher_cert: dev 7f3a"), ErrBounds},
		{"another namespace's kind", edit(t, "kind: snowflake/warehouse,", "kind: k8s/deployment,"), ErrKind},
		{"an unprefixed kind", edit(t, "kind: snowflake/warehouse,", "kind: warehouse,"), ErrKind},
		{"a two-prefix kind", edit(t, "kind: snowflake/warehouse,", "kind: snowflake/a/b,"), ErrKind},
		{"a like that isn't core", edit(t, "like: database", "like: snowflake/db"), ErrKind},
		{"a repeated kind", edit(t, "like: database}", "like: database}\n  - {kind: snowflake/warehouse}"), ErrRepeated},
		{"an action on an unlisted kind", edit(t, "kinds: [snowflake/warehouse]", "kinds: [host]"), ErrKind},
		{"an action on no kind", edit(t, "kinds: [snowflake/warehouse]", "kinds: []"), ErrMissing},
		{"a bad action id", edit(t, "id: resume", "id: Resume"), ErrBounds},
		{"a repeated action", edit(t, "kinds: [snowflake/warehouse]}", "kinds: [snowflake/warehouse]}\n  - {id: resume, title: t, changes: c, kinds: [snowflake/warehouse]}"), ErrRepeated},
		{"a null field", edit(t, "name: Snowflake", "name:"), ErrBounds},
		{"too many endpoints", edit(t, "network: [configured by user]", "network: ["+strings.Repeat("a, ", maxNetwork)+"a]"), ErrBounds},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(c.in)
			if !errors.Is(err, c.want) {
				t.Errorf("Parse: %v; want %v", err, c.want)
			}
		})
	}
}

func TestErrorsNameTheLineAndFieldOnly(t *testing.T) {
	_, err := Parse(edit(t, "publisher_cert: dev-7f3a91c2", "publisher_cert: secret value"))
	if err == nil || strings.Contains(err.Error(), "secret") || !strings.Contains(err.Error(), "publisher_cert") ||
		!strings.Contains(err.Error(), "line 13") {
		t.Errorf("error %q; want the line and field and never the value", err)
	}
}

func TestCoreKindsAllowedWithoutANamespace(t *testing.T) {
	m, err := Parse(edit(t, "- {kind: snowflake/warehouse,", "- {kind: database}\n  - {kind: snowflake/warehouse,"))
	if err != nil || len(m.Kinds) != 2 {
		t.Fatalf("a core kind beside the namespace's: %+v, %v", m.Kinds, err)
	}
}

func TestNamespaceRule(t *testing.T) {
	for ns, want := range map[string]bool{
		"acme": true, "a1-b": true, "ab": true, strings.Repeat("a", 32): true,
		"a": false, strings.Repeat("a", 33): false, "1ab": false, "Acme": false, "a_b": false, "a.b": false, "": false,
	} {
		if got := ValidNamespace(ns); got != want {
			t.Errorf("ValidNamespace(%q) = %v; want %v", ns, got, want)
		}
	}
	for _, ns := range []string{"k8s", "wayseer", "localhost", "prometheus", "sql", "proxmox", "docker"} {
		if !Reserved(ns) {
			t.Errorf("%s isn't reserved", ns)
		}
	}
}

func TestActionIDRule(t *testing.T) {
	for id, want := range map[string]bool{
		"resume": true, "scale-up": true, "r2": true, strings.Repeat("a", 64): true,
		"": false, "2r": false, "-r": false, "Resume": false, "re_sume": false, strings.Repeat("a", 65): false,
	} {
		if got := ValidActionID(id); got != want {
			t.Errorf("ValidActionID(%q) = %v; want %v", id, got, want)
		}
	}
}
