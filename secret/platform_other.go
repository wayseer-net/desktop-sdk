//go:build !darwin && !windows

package secret

func platform() Store { return Freedesktop{} }
