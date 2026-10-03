package wsmod

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"strings"
)

// Read reads a package in format f from r, in memory only, refusing anything but the three
// files. At most MaxArchive bytes are read, compressed or not.
func Read(r io.Reader, f Format) (Package, error) {
	if f == Zip {
		return readZip(r)
	}
	return readTarGz(r)
}

type entryType int

const (
	regular entryType = iota
	link
	directory
	irregular
)

// collector checks each entry before its body is read, and gathers the bodies.
type collector struct {
	pkg  Package
	f    Format
	seen map[string]bool
}

func newCollector(f Format) *collector { return &collector{f: f, seen: map[string]bool{}} }

// admit checks an entry's type, name and size, returning where its body goes and its limit.
func (c *collector) admit(t entryType, name string, size int64) (*[]byte, int64, error) {
	switch t {
	case link:
		return nil, 0, ErrLink
	case directory:
		return nil, 0, ErrDirectory
	case irregular:
		return nil, 0, ErrNotRegular
	}
	if err := checkName(name); err != nil {
		return nil, 0, err
	}
	dst, limit := c.slot(name)
	switch {
	case dst == nil:
		return nil, 0, ErrExtra
	case c.seen[name]:
		return nil, 0, fmt.Errorf("%w: %s", ErrRepeated, name)
	case size < 0 || size > limit:
		return nil, 0, fmt.Errorf("%w: %s", ErrEntrySize, name)
	}
	c.seen[name] = true
	return dst, limit, nil
}

func (c *collector) slot(name string) (*[]byte, int64) {
	switch name {
	case c.f.Executable():
		return &c.pkg.Module, limits.module
	case ManifestName:
		return &c.pkg.Manifest, limits.manifest
	case SignatureName:
		return &c.pkg.Signature, limits.signature
	}
	return nil, 0
}

func (c *collector) done() (Package, error) {
	for _, name := range []string{c.f.Executable(), ManifestName, SignatureName} {
		if !c.seen[name] {
			return Package{}, fmt.Errorf("%w: %s", ErrMissing, name)
		}
	}
	return c.pkg, nil
}

// checkName refuses any name that is more than a bare file name.
func checkName(name string) error {
	switch {
	case strings.Contains(name, `\`):
		return ErrBackslash
	case strings.HasPrefix(name, "/"):
		return ErrAbsolute
	case len(name) >= 2 && name[1] == ':' && isLetter(name[0]):
		return ErrDriveLetter
	case strings.Contains(name, "/"):
		return ErrPath
	}
	return nil
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// body reads at most limit bytes into dst, refusing a longer entry whatever its header said.
func body(r io.Reader, dst *[]byte, limit int64) error {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return err
	}
	if int64(len(b)) > limit {
		return ErrEntrySize
	}
	*dst = b
	return nil
}

// capped reads at most its limit, then fails with ErrArchiveSize and remembers it did.
type capped struct {
	r    io.Reader
	left int64
	over bool
}

func (c *capped) Read(p []byte) (int, error) {
	if int64(len(p)) > c.left+1 {
		p = p[:c.left+1]
	}
	n, err := c.r.Read(p)
	c.left -= int64(n)
	if c.left < 0 {
		c.over = true
		return 0, ErrArchiveSize
	}
	return n, err
}

// formatError keeps the package's own refusals and turns any other failure into ErrFormat,
// without the underlying text.
func formatError(err error, caps ...*capped) error {
	for _, c := range caps {
		if c.over {
			return ErrArchiveSize
		}
	}
	if isRefusal(err) {
		return err
	}
	return ErrFormat
}

func isRefusal(err error) bool {
	for _, r := range []error{
		ErrLink, ErrDirectory, ErrNotRegular, ErrPath, ErrAbsolute, ErrDriveLetter,
		ErrBackslash, ErrExtra, ErrMissing, ErrRepeated, ErrEntrySize, ErrArchiveSize,
	} {
		if errors.Is(err, r) {
			return true
		}
	}
	return false
}

func readTarGz(r io.Reader) (Package, error) {
	raw := &capped{r: r, left: limits.archive}
	gz, err := gzip.NewReader(raw)
	if err != nil {
		return Package{}, formatError(err, raw)
	}
	plain := &capped{r: gz, left: limits.archive}
	pkg, err := readTar(tar.NewReader(plain))
	if err != nil {
		return Package{}, formatError(err, raw, plain)
	}
	return pkg, nil
}

func readTar(tr *tar.Reader) (Package, error) {
	c := newCollector(TarGz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return c.done()
		}
		if err != nil && !errors.Is(err, tar.ErrInsecurePath) {
			return Package{}, err
		}
		dst, limit, err := c.admit(tarType(h.Typeflag), h.Name, h.Size)
		if err != nil {
			return Package{}, err
		}
		if err := body(tr, dst, limit); err != nil {
			return Package{}, err
		}
	}
}

func tarType(flag byte) entryType {
	switch flag {
	case tar.TypeReg:
		return regular
	case tar.TypeSymlink, tar.TypeLink:
		return link
	case tar.TypeDir:
		return directory
	}
	return irregular
}

func readZip(r io.Reader) (Package, error) {
	raw := &capped{r: r, left: limits.archive}
	data, err := io.ReadAll(raw)
	if err != nil {
		return Package{}, formatError(err, raw)
	}
	if err := checkDirectory(data); err != nil {
		return Package{}, err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		return Package{}, ErrFormat
	}
	c := newCollector(Zip)
	for _, zf := range zr.File {
		if err := readZipFile(c, zf); err != nil {
			return Package{}, formatError(err)
		}
	}
	return c.done()
}

func readZipFile(c *collector, zf *zip.File) error {
	size := int64(math.MaxInt64)
	if zf.UncompressedSize64 < math.MaxInt64 {
		size = int64(zf.UncompressedSize64)
	}
	dst, limit, err := c.admit(zipType(zf), zf.Name, size)
	if err != nil {
		return err
	}
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	return body(rc, dst, limit)
}

func zipType(zf *zip.File) entryType {
	m := zf.Mode()
	switch {
	case m&fs.ModeSymlink != 0:
		return link
	case m.IsDir() || strings.HasSuffix(zf.Name, "/"):
		return directory
	case m.Type() != 0:
		return irregular
	}
	return regular
}

// The end of a zip's central directory, which must close the file with no comment.
const (
	eocdLen       = 22
	eocdSignature = 0x06054b50
	eocdEntries   = 10
	maxListed     = 16
)

// checkDirectory refuses a zip whose directory lists many entries before Go's reader allocates
// a record for each; a few extra still reach the per-entry checks, which name the problem.
func checkDirectory(data []byte) error {
	if len(data) < eocdLen {
		return ErrFormat
	}
	end := data[len(data)-eocdLen:]
	if binary.LittleEndian.Uint32(end) != eocdSignature {
		return ErrFormat
	}
	if binary.LittleEndian.Uint16(end[eocdEntries:]) > maxListed {
		return ErrExtra
	}
	return nil
}
