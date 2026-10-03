package secret

import (
	"bytes"
	"encoding/binary"
	"unicode/utf16"
)

// decodeBlob reads a Credential Manager blob. cmdkey and the Windows dialogs store UTF-16LE;
// other tools store UTF-8, which never holds a zero byte.
func decodeBlob(b []byte) string {
	if len(b)%2 != 0 || bytes.IndexByte(b, 0) < 0 {
		return string(b)
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	return string(utf16.Decode(u))
}
