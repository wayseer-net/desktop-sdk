package module

import (
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

type testOptions struct {
	Path     string        `yaml:"path"`
	Interval time.Duration `yaml:"interval"`
	Limits   struct {
		Rows int `yaml:"rows"`
	} `yaml:"limits"`
	Columns []struct {
		Name string `yaml:"name"`
	} `yaml:"columns"`
	Labels map[string]string `yaml:"labels"`
	Common `yaml:",inline"`
	server `yaml:",inline"`
}

// server is unexported, as a module's own shared options usually are.
type server struct {
	URL string `yaml:"url"`
}

type Common struct {
	Timeout time.Duration `yaml:"timeout"`
}

func optionsConfig(t *testing.T, src string) Config {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatal(err)
	}
	var n yaml.Node
	if len(doc.Content) > 0 {
		n = *doc.Content[0]
	}
	return Config{Name: "test", Line: 1, Options: n}
}

func TestDecodeReadsKnownOptions(t *testing.T) {
	c := optionsConfig(t, `
path: /tmp/x
interval: 5s
timeout: 2s
limits: {rows: 10}
columns: [{name: a}]
labels: {anything: goes}
url: http://x
`)
	var o testOptions
	if err := c.Decode(&o); err != nil {
		t.Fatal(err)
	}
	if o.Path != "/tmp/x" || o.Interval != 5*time.Second || o.Timeout != 2*time.Second ||
		o.Limits.Rows != 10 || o.Columns[0].Name != "a" || o.Labels["anything"] != "goes" || o.URL != "http://x" {
		t.Errorf("decoded %+v", o)
	}
}

func TestDecodeRejectsUnknownOptionsWithLines(t *testing.T) {
	cases := map[string]string{
		"pth: /tmp/x":                   `line 1: unknown option "pth"`,
		"limits:\n  rows: 1\n  cols: 2": `line 3: unknown option "limits.cols"`,
		"columns:\n  - nme: a":          `line 2: unknown option "columns[0].nme"`,
		"interval: soon":                "line 1",
		"- a":                           "line 1: options must be a mapping",
	}
	for src, want := range cases {
		var o testOptions
		err := optionsConfig(t, src).Decode(&o)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v, want %q", src, err, want)
		}
	}
}

func TestDecodeEmptyOptionsKeepsDefaults(t *testing.T) {
	o := testOptions{Path: "default"}
	if err := (Config{Name: "test"}).Decode(&o); err != nil || o.Path != "default" {
		t.Errorf("o = %+v, err = %v", o, err)
	}
}
