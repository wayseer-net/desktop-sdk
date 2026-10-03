package manifest

import (
	"bytes"
	"reflect"
	"testing"
)

// FuzzParse checks Parse never panics and accepts only what Marshal gives back unchanged.
func FuzzParse(f *testing.F) {
	f.Add(example(f))
	f.Add([]byte("id: a/b\n&x k: *x\n"))
	f.Add([]byte("---\n---\n"))
	f.Fuzz(func(t *testing.T, in []byte) {
		m, err := Parse(in)
		if err != nil {
			return
		}
		out, err := Marshal(m)
		if err != nil {
			t.Fatalf("Parse accepted what Marshal refuses: %v", err)
		}
		again, err := Parse(out)
		if err != nil || !reflect.DeepEqual(again, m) {
			t.Fatalf("Marshal gave %q, which parses as %+v, %v; want %+v", out, again, err, m)
		}
		if out2, _ := Marshal(again); !bytes.Equal(out, out2) {
			t.Fatalf("Marshal isn't stable: %q then %q", out, out2)
		}
	})
}

// FuzzFill checks Fill never panics and gives only bytes Parse reads as the filled manifest.
func FuzzFill(f *testing.F) {
	f.Add(example(f))
	f.Add([]byte(unfilled))
	f.Add([]byte(unfilled + "os: [x]\n"))
	f.Fuzz(func(t *testing.T, in []byte) {
		out, m, err := Fill(in, platformFields)
		if err != nil {
			return
		}
		again, err := Parse(out)
		if err != nil || !reflect.DeepEqual(again, m) {
			t.Fatalf("Fill gave %q, which parses as %+v, %v; want %+v", out, again, err, m)
		}
	})
}
