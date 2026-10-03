package wsmod

import (
	"archive/tar"
	"bytes"
	"reflect"
	"testing"
)

func FuzzArchiveTarGz(f *testing.F) { fuzzArchive(f, TarGz) }

func FuzzArchiveZip(f *testing.F) { fuzzArchive(f, Zip) }

// fuzzArchive checks Read never panics, and that what it accepts writes back and reads the same.
func fuzzArchive(f *testing.F, format Format) {
	f.Add(written(f, format, sample()))
	f.Add(archiveOf(f, format, append(three(format), reg("extra", "x"))...))
	f.Add(archiveOf(f, format, entry{format.Executable(), tar.TypeSymlink, ""}))
	f.Add(archiveOf(f, format, reg("../"+format.Executable(), "x")))
	f.Fuzz(func(t *testing.T, in []byte) {
		p, err := Read(bytes.NewReader(in), format)
		if err != nil {
			return
		}
		var out bytes.Buffer
		if err := Write(&out, format, p); err != nil {
			t.Fatalf("Read accepted what Write refuses: %v", err)
		}
		again, err := Read(bytes.NewReader(out.Bytes()), format)
		if err != nil || !reflect.DeepEqual(again, p) {
			t.Fatalf("a rewritten package reads as %+v, %v; want %+v", again, err, p)
		}
	})
}
