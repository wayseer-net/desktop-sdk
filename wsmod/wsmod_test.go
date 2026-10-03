package wsmod

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"io/fs"
	"os"
	"reflect"
	"strings"
	"testing"
	"wayseer/pkg/sdk/manifest"
)

var formats = []Format{TarGz, Zip}

func sample() Package {
	return Package{
		Module:    []byte("\x7fELF a module's bytes"),
		Manifest:  []byte("id: acme/snowflake\n"),
		Signature: []byte("WAYSEER-SIG-1.chain.sig\n"),
	}
}

func written(t testing.TB, f Format, p Package) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := Write(&b, f, p); err != nil {
		t.Fatalf("Write(%v): %v", f, err)
	}
	return b.Bytes()
}

// withLimits shrinks the size limits for one test, so bombs stay small.
func withLimits(t *testing.T, l limitSet) {
	t.Helper()
	old := limits
	limits = l
	t.Cleanup(func() { limits = old })
}

func TestRoundTrip(t *testing.T) {
	for _, f := range formats {
		t.Run(f.String(), func(t *testing.T) {
			want := sample()
			got, err := Read(bytes.NewReader(written(t, f, want)), f)
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Read gave %+v, want %+v", got, want)
			}
		})
	}
}

func TestWriteIsStable(t *testing.T) {
	for _, f := range formats {
		if a, b := written(t, f, sample()), written(t, f, sample()); !bytes.Equal(a, b) {
			t.Errorf("%v: two writes of one package differ", f)
		}
	}
}

func TestWriteRefusesOversizedEntries(t *testing.T) {
	withLimits(t, limitSet{module: 8, manifest: 8, signature: 8, archive: 1 << 20})
	for _, f := range formats {
		p := sample()
		if err := Write(&bytes.Buffer{}, f, p); !errors.Is(err, ErrEntrySize) {
			t.Errorf("%v: Write = %v, want ErrEntrySize", f, err)
		}
	}
}

func TestManifestLimitMatches(t *testing.T) {
	if MaxManifest != manifest.MaxSize {
		t.Fatalf("MaxManifest = %d, manifest.MaxSize = %d", MaxManifest, manifest.MaxSize)
	}
}

func TestExecutableName(t *testing.T) {
	if TarGz.Executable() != "module" || Zip.Executable() != "module.exe" {
		t.Fatalf("executables: %q, %q", TarGz.Executable(), Zip.Executable())
	}
}

func TestFormatOf(t *testing.T) {
	for name, want := range map[string]Format{
		"acme-snowflake-1.4.0-linux-amd64.wsmod.tar.gz": TarGz,
		"acme-snowflake-1.4.0-windows-amd64.wsmod.zip":  Zip,
	} {
		if got, err := FormatOf(name); err != nil || got != want {
			t.Errorf("FormatOf(%q) = %v, %v; want %v", name, got, err, want)
		}
	}
	for _, name := range []string{"a.tar.gz", "a.zip", "a.wsmod", "a.wsmod.tar", ""} {
		if _, err := FormatOf(name); !errors.Is(err, ErrFormat) {
			t.Errorf("FormatOf(%q) = %v, want ErrFormat", name, err)
		}
	}
}

// entry is one crafted archive member; body is written as given, whatever the header says.
type entry struct {
	name string
	kind byte // a tar type flag; zip maps it to a mode
	body string
}

func reg(name, body string) entry { return entry{name, tar.TypeReg, body} }

func three(f Format) []entry {
	return []entry{reg(f.Executable(), "exe"), reg("manifest.yaml", "m"), reg("signature", "s")}
}

func tarGzOf(t testing.TB, es ...entry) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for _, e := range es {
		h := &tar.Header{Name: e.name, Typeflag: e.kind, Mode: 0o644, Size: int64(len(e.body))}
		if e.kind == tar.TypeSymlink || e.kind == tar.TypeLink {
			h.Linkname, h.Size = "/etc/passwd", 0
		}
		if e.kind != tar.TypeReg {
			h.Size = 0
		}
		must(t, tw.WriteHeader(h))
		if h.Size > 0 {
			_, err := tw.Write([]byte(e.body))
			must(t, err)
		}
	}
	must(t, tw.Close())
	must(t, gz.Close())
	return b.Bytes()
}

func zipOf(t testing.TB, es ...entry) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, e := range es {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		h.SetMode(zipMode(e.kind))
		w, err := zw.CreateHeader(h)
		must(t, err)
		_, err = w.Write([]byte(e.body))
		must(t, err)
	}
	must(t, zw.Close())
	return b.Bytes()
}

func zipMode(kind byte) fs.FileMode {
	switch kind {
	case tar.TypeSymlink, tar.TypeLink:
		return fs.ModeSymlink | 0o777
	case tar.TypeDir:
		return fs.ModeDir | 0o755
	case tar.TypeFifo:
		return fs.ModeNamedPipe | 0o644
	}
	return 0o644
}

func archiveOf(t testing.TB, f Format, es ...entry) []byte {
	if f == Zip {
		return zipOf(t, es...)
	}
	return tarGzOf(t, es...)
}

func must(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestRefusals(t *testing.T) {
	for _, f := range formats {
		exe := f.Executable()
		ok := three(f)
		for name, c := range map[string]struct {
			entries []entry
			want    error
		}{
			"symlink":       {append(ok[1:], entry{exe, tar.TypeSymlink, ""}), ErrLink},
			"directory":     {append(ok, entry{"bin/", tar.TypeDir, ""}), ErrDirectory},
			"fifo":          {append(ok[1:], entry{exe, tar.TypeFifo, ""}), ErrNotRegular},
			"path":          {append(ok[1:], reg("bin/"+exe, "exe")), ErrPath},
			"dot path":      {append(ok[1:], reg("./"+exe, "exe")), ErrPath},
			"climbing":      {append(ok[1:], reg("../../"+exe, "exe")), ErrPath},
			"absolute":      {append(ok[1:], reg("/"+exe, "exe")), ErrAbsolute},
			"drive letter":  {append(ok[1:], reg("C:"+exe, "exe")), ErrDriveLetter},
			"backslash":     {append(ok[1:], reg(`bin\`+exe, "exe")), ErrBackslash},
			"extra":         {append(ok, reg("README", "hi")), ErrExtra},
			"other exe":     {append(ok[1:], reg(other(f).Executable(), "exe")), ErrExtra},
			"empty name":    {append(ok, reg("", "x")), ErrExtra},
			"missing":       {ok[1:], ErrMissing},
			"empty archive": {nil, ErrMissing},
			"repeated":      {append(ok, reg("signature", "s2")), ErrRepeated},
		} {
			t.Run(f.String()+"/"+name, func(t *testing.T) {
				_, err := Read(bytes.NewReader(archiveOf(t, f, c.entries...)), f)
				if !errors.Is(err, c.want) {
					t.Fatalf("Read = %v, want %v", err, c.want)
				}
			})
		}
	}
}

func other(f Format) Format {
	if f == Zip {
		return TarGz
	}
	return Zip
}

func TestErrorsNeverEchoNames(t *testing.T) {
	const secret = "licence-KEY-abc123"
	for _, f := range formats {
		data := archiveOf(t, f, append(three(f), reg(secret, "x"))...)
		if _, err := Read(bytes.NewReader(data), f); err == nil || strings.Contains(err.Error(), secret) {
			t.Errorf("%v: Read = %v; want an error without the entry's name", f, err)
		}
	}
}

func TestEntryOverLimit(t *testing.T) {
	withLimits(t, limitSet{module: 4, manifest: 4, signature: 4, archive: 1 << 20})
	for _, f := range formats {
		es := three(f)
		es[1].body = "12345"
		if _, err := Read(bytes.NewReader(archiveOf(t, f, es...)), f); !errors.Is(err, ErrEntrySize) {
			t.Errorf("%v: Read = %v, want ErrEntrySize", f, err)
		}
	}
}

func TestArchiveOverLimit(t *testing.T) {
	withLimits(t, limitSet{module: 1 << 20, manifest: 64, signature: 64, archive: 256})
	for _, f := range formats {
		es := three(f)
		es[0].body = noise(4096)
		if _, err := Read(bytes.NewReader(archiveOf(t, f, es...)), f); !errors.Is(err, ErrArchiveSize) {
			t.Errorf("%v: Read = %v, want ErrArchiveSize", f, err)
		}
	}
}

// noise is n bytes that don't compress, so the compressed archive is large too.
func noise(n int) string {
	b := make([]byte, n)
	x := uint32(2463534242)
	for i := range b {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		b[i] = byte(x)
	}
	return string(b)
}

// A gzip bomb: a tar header promising a huge module, with gigabytes of zeros behind it.
func TestGzipBombStopsAtTheHeader(t *testing.T) {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	must(t, tw.WriteHeader(&tar.Header{Name: "module", Typeflag: tar.TypeReg, Mode: 0o755, Size: 1 << 40}))
	zeros := make([]byte, 1<<20)
	for range 8 {
		_, err := tw.Write(zeros)
		must(t, err)
	}
	must(t, gz.Flush())
	in := &countingReader{r: bytes.NewReader(b.Bytes())}
	if _, err := Read(in, TarGz); !errors.Is(err, ErrEntrySize) {
		t.Fatalf("Read = %v, want ErrEntrySize", err)
	}
	if in.n > 64<<10 {
		t.Fatalf("Read took %d compressed bytes before refusing", in.n)
	}
}

// A gzip bomb through PAX headers, which the tar reader reads itself: the archive limit stops it.
func TestGzipBombStopsAtTheArchiveLimit(t *testing.T) {
	withLimits(t, limitSet{module: 64, manifest: 64, signature: 64, archive: 4096})
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	h := &tar.Header{
		Name: "module", Typeflag: tar.TypeReg, Mode: 0o755, Format: tar.FormatPAX,
		PAXRecords: map[string]string{"comment": strings.Repeat("z", 512<<10)},
	}
	must(t, tw.WriteHeader(h))
	must(t, tw.Close())
	must(t, gz.Close())
	if _, err := Read(bytes.NewReader(b.Bytes()), TarGz); !errors.Is(err, ErrArchiveSize) {
		t.Fatalf("Read = %v, want ErrArchiveSize", err)
	}
}

// A zip bomb: the header's size is checked before anything is inflated.
func TestZipBombStopsAtTheHeader(t *testing.T) {
	withLimits(t, limitSet{module: 1024, manifest: 64, signature: 64, archive: 1 << 20})
	es := three(Zip)
	es[0].body = strings.Repeat("\x00", 1<<20)
	if _, err := Read(bytes.NewReader(zipOf(t, es...)), Zip); !errors.Is(err, ErrEntrySize) {
		t.Fatalf("Read = %v, want ErrEntrySize", err)
	}
}

// A zip whose header understates an entry's size: reading stops at the limit, not the data.
func TestZipBombWithLyingHeader(t *testing.T) {
	withLimits(t, limitSet{module: 1024, manifest: 64, signature: 64, archive: 1 << 20})
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, e := range three(Zip)[1:] {
		w, err := zw.Create(e.name)
		must(t, err)
		_, err = w.Write([]byte(e.body))
		must(t, err)
	}
	w, err := zw.CreateRaw(&zip.FileHeader{
		Name: "module.exe", Method: zip.Deflate,
		CompressedSize64: uint64(len(deflated(t, 1<<22))), UncompressedSize64: 10,
	})
	must(t, err)
	_, err = w.Write(deflated(t, 1<<22))
	must(t, err)
	must(t, zw.Close())
	if _, err := Read(bytes.NewReader(b.Bytes()), Zip); !errors.Is(err, ErrEntrySize) && !errors.Is(err, ErrFormat) {
		t.Fatalf("Read = %v, want ErrEntrySize or ErrFormat", err)
	}
}

func deflated(t testing.TB, n int) []byte {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "x", Method: zip.Deflate})
	must(t, err)
	_, err = w.Write(make([]byte, n))
	must(t, err)
	must(t, zw.Close())
	r, err := zip.NewReader(bytes.NewReader(b.Bytes()), int64(b.Len()))
	must(t, err)
	off, err := r.File[0].DataOffset()
	must(t, err)
	return b.Bytes()[off : off+int64(r.File[0].CompressedSize64)]
}

// A zip may claim thousands of entries; its directory is refused before Go's reader lists them.
func TestZipWithManyEntries(t *testing.T) {
	es := three(Zip)
	for i := range 40 {
		es = append(es, reg(strings.Repeat("x", i+1), ""))
	}
	if _, err := Read(bytes.NewReader(zipOf(t, es...)), Zip); !errors.Is(err, ErrExtra) {
		t.Fatalf("Read = %v, want ErrExtra", err)
	}
}

func TestNotAnArchive(t *testing.T) {
	for _, f := range formats {
		for _, data := range [][]byte{nil, []byte("not an archive"), archiveOf(t, other(f), three(other(f))...)} {
			if _, err := Read(bytes.NewReader(data), f); !errors.Is(err, ErrFormat) {
				t.Errorf("%v: Read(%.16q) = %v, want ErrFormat", f, data, err)
			}
		}
	}
}

func TestReadWritesNothingToDisk(t *testing.T) {
	tmp, cwd := t.TempDir(), t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Chdir(cwd)
	for _, f := range formats {
		if _, err := Read(bytes.NewReader(written(t, f, sample())), f); err != nil {
			t.Fatal(err)
		}
		if _, err := Read(bytes.NewReader(archiveOf(t, f, reg("../../escape", "x"))), f); err == nil {
			t.Fatal("Read accepted a climbing name")
		}
	}
	for _, dir := range []string{tmp, cwd} {
		if es, _ := os.ReadDir(dir); len(es) != 0 {
			t.Fatalf("Read wrote %d entries into %s", len(es), dir)
		}
	}
}

type countingReader struct {
	r interface{ Read([]byte) (int, error) }
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}
