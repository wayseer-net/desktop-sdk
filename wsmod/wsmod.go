// Package wsmod reads and writes module packages: a .wsmod.tar.gz or .wsmod.zip holding exactly
// the executable, manifest.yaml and signature, as regular files with no paths.
package wsmod

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// The size limits, in bytes. Every entry is checked against its limit before it is read.
const (
	MaxModule    = 256 << 20
	MaxManifest  = 64 << 10 // manifest.MaxSize; a test keeps them equal
	MaxSignature = 16 << 10
	MaxArchive   = MaxModule + 1<<20
)

// The names of a package's entries, besides the executable (see Format.Executable).
const (
	ManifestName  = "manifest.yaml"
	SignatureName = "signature"
)

// The reasons an archive is refused. None holds an entry's name or contents.
var (
	ErrFormat      = errors.New("not a module package")
	ErrLink        = errors.New("the package holds a link")
	ErrDirectory   = errors.New("the package holds a directory")
	ErrNotRegular  = errors.New("the package holds an entry that isn't a regular file")
	ErrPath        = errors.New("the package holds an entry with a path")
	ErrAbsolute    = errors.New("the package holds an entry with an absolute path")
	ErrDriveLetter = errors.New("the package holds an entry with a drive letter")
	ErrBackslash   = errors.New("the package holds an entry with a backslash")
	ErrExtra       = errors.New("the package holds an unexpected entry")
	ErrMissing     = errors.New("the package is missing an entry")
	ErrRepeated    = errors.New("the package holds an entry twice")
	ErrEntrySize   = errors.New("a package entry is too large")
	ErrArchiveSize = errors.New("the package is too large")
)

// limitSet holds the limits in force; tests shrink them.
type limitSet struct{ module, manifest, signature, archive int64 }

var limits = limitSet{MaxModule, MaxManifest, MaxSignature, MaxArchive}

// Format is a package's archive format: gzipped tar, or zip on Windows.
type Format int

// The formats.
const (
	TarGz Format = iota
	Zip
)

func (f Format) String() string {
	if f == Zip {
		return "zip"
	}
	return "tar.gz"
}

// Executable is the executable's entry name: module.exe in a zip, which is for Windows.
func (f Format) Executable() string {
	if f == Zip {
		return "module.exe"
	}
	return "module"
}

// FormatOf picks the format from a package file's name.
func FormatOf(name string) (Format, error) {
	switch {
	case strings.HasSuffix(name, ".wsmod.tar.gz"):
		return TarGz, nil
	case strings.HasSuffix(name, ".wsmod.zip"):
		return Zip, nil
	}
	return 0, fmt.Errorf("%w: the name ends in neither .wsmod.tar.gz nor .wsmod.zip", ErrFormat)
}

// Package is a package's three files, as bytes.
type Package struct {
	Module    []byte
	Manifest  []byte
	Signature []byte
}

type file struct {
	name  string
	data  []byte
	mode  int64
	limit int64
}

func (p Package) files(f Format) []file {
	return []file{
		{f.Executable(), p.Module, 0o755, limits.module},
		{ManifestName, p.Manifest, 0o644, limits.manifest},
		{SignatureName, p.Signature, 0o644, limits.signature},
	}
}

// Write writes p as an archive in format f, the same bytes every time.
func Write(w io.Writer, f Format, p Package) error {
	files := p.files(f)
	for _, fl := range files {
		if int64(len(fl.data)) > fl.limit {
			return fmt.Errorf("%w: %s", ErrEntrySize, fl.name)
		}
	}
	if f == Zip {
		return writeZip(w, files)
	}
	return writeTarGz(w, files)
}

func writeTarGz(w io.Writer, files []file) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	for _, fl := range files {
		h := &tar.Header{
			Name: fl.name, Typeflag: tar.TypeReg, Mode: fl.mode, Size: int64(len(fl.data)),
			ModTime: time.Unix(0, 0), Format: tar.FormatUSTAR,
		}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if _, err := tw.Write(fl.data); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func writeZip(w io.Writer, files []file) error {
	zw := zip.NewWriter(w)
	for _, fl := range files {
		h := &zip.FileHeader{Name: fl.name, Method: zip.Deflate}
		h.SetMode(0o644)
		if fl.mode == 0o755 {
			h.SetMode(0o755)
		}
		fw, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		if _, err := fw.Write(fl.data); err != nil {
			return err
		}
	}
	return zw.Close()
}
