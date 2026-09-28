package netfail

import (
	"errors"
	"fmt"
	"net"
	"testing"
)

func TestADialToAClosedPortIsRefused(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	_, err = net.Dial("tcp", addr)
	if err == nil || !Refused(fmt.Errorf("wrapped: %w", err)) {
		t.Errorf("Refused(%v) = false; want true", err)
	}
}

func TestOtherErrorsAreNotRefused(t *testing.T) {
	for _, err := range []error{nil, errors.New("connection refused"), &net.DNSError{Err: "no such host"}} {
		if Refused(err) {
			t.Errorf("Refused(%v) = true", err)
		}
	}
}
