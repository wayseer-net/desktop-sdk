package netfail

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// Windows reports a refused connection as WSAECONNREFUSED, which syscall.ECONNREFUSED is not.
var refusedErrnos = []error{syscall.ECONNREFUSED, windows.WSAECONNREFUSED}
