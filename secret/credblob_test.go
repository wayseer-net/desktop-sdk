package secret

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

func utf16le(s string) []byte {
	var b []byte
	for _, u := range utf16.Encode([]rune(s)) {
		b = binary.LittleEndian.AppendUint16(b, u)
	}
	return b
}

func TestCredentialBlobsDecodeAsEitherToolStoresThem(t *testing.T) {
	for _, s := range []string{"sk-ant-api03-abc", "pässwörd", "ab"} {
		if got := decodeBlob(utf16le(s)); got != s {
			t.Errorf("UTF-16 %q decodes as %q", s, got)
		}
		if got := decodeBlob([]byte(s)); got != s {
			t.Errorf("UTF-8 %q decodes as %q", s, got)
		}
	}
}
