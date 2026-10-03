package secret

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

const credTypeGeneric = 1 // CRED_TYPE_GENERIC, as cmdkey /generic stores

var (
	advapi32 = windows.NewLazySystemDLL("advapi32.dll")
	credRead = advapi32.NewProc("CredReadW")
	credFree = advapi32.NewProc("CredFree")
)

// credential is Windows' CREDENTIALW.
type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

func platform() Store { return CredentialManager{} }

// CredentialManager reads generic credentials whose target is the service and whose user is
// the account.
type CredentialManager struct{}

// Get reads the credential stored for service, if its user is account.
func (CredentialManager) Get(service, account string) (string, error) {
	target, err := windows.UTF16PtrFromString(service)
	if err != nil {
		return "", err
	}
	var c *credential
	if ok, _, err := credRead.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&c))); ok == 0 {
		return "", credError(err)
	}
	defer func() { _, _, _ = credFree.Call(uintptr(unsafe.Pointer(c))) }()
	if windows.UTF16PtrToString(c.UserName) != account {
		return "", ErrNotFound
	}
	return decodeBlob(unsafe.Slice(c.CredentialBlob, c.CredentialBlobSize)), nil
}

// credError names why CredReadW failed; Windows' messages hold no credential.
func credError(err error) error {
	switch {
	case errors.Is(err, windows.ERROR_NOT_FOUND):
		return ErrNotFound
	case errors.Is(err, windows.ERROR_NO_SUCH_LOGON_SESSION):
		return NoKeyring("the Credential Manager has no logon session")
	}
	return err
}
