// Package netfail recognises network failures the same way on every platform, for modules
// that explain them in their own words.
package netfail

import "errors"

// Refused reports whether err is a connection the other end refused: nothing listens there.
func Refused(err error) bool {
	for _, errno := range refusedErrnos {
		if errors.Is(err, errno) {
			return true
		}
	}
	return false
}
