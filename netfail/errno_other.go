//go:build !windows

package netfail

import "syscall"

var refusedErrnos = []error{syscall.ECONNREFUSED}
