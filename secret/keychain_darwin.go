package secret

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

const (
	security                = "/System/Library/Frameworks/Security.framework/Security"
	errSecItemNotFound      = -25300
	errSecNoDefaultKeychain = -25307
)

// sec is the part of the Security framework that reads a generic password.
var sec struct {
	findGenericPassword func(keychains uintptr, serviceLen uint32, service *byte, accountLen uint32, account *byte, passwordLen *uint32, password *unsafe.Pointer, item uintptr) int32
	freeContent         func(attrs uintptr, data unsafe.Pointer) int32
}

var loadSecurity = sync.OnceValue(func() error {
	lib, err := purego.Dlopen(security, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return err
	}
	purego.RegisterLibFunc(&sec.findGenericPassword, lib, "SecKeychainFindGenericPassword")
	purego.RegisterLibFunc(&sec.freeContent, lib, "SecKeychainItemFreeContent")
	return nil
})

func platform() Store { return Keychain{} }

// Keychain reads generic passwords, as `security add-generic-password -s service -a account`
// stores them, from the login keychain.
type Keychain struct{}

// Get reads the password for service and account; macOS may ask the owner to allow it.
func (Keychain) Get(service, account string) (string, error) {
	if err := loadSecurity(); err != nil {
		return "", NoKeyring("the Security framework did not load")
	}
	s, a := append([]byte(service), 0), append([]byte(account), 0)
	var n uint32
	var data unsafe.Pointer
	switch status := sec.findGenericPassword(0, uint32(len(service)), &s[0], uint32(len(account)), &a[0], &n, &data, 0); status {
	case 0:
	case errSecItemNotFound:
		return "", ErrNotFound
	case errSecNoDefaultKeychain:
		return "", NoKeyring("there is no default keychain")
	default:
		return "", fmt.Errorf("the keychain: OSStatus %d", status)
	}
	defer sec.freeContent(0, data)
	return string(unsafe.Slice((*byte)(data), n)), nil
}
