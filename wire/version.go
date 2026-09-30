package wire

import (
	"fmt"
	pb "mindseye/pkg/sdk/proto/modulev1"
)

// Version is a contract version: a new major breaks compatibility, a new minor only adds.
type Version struct{ Major, Minor uint32 }

// Protocol is the contract version this build speaks.
var Protocol = Version{Major: 1, Minor: 3}

func (v Version) String() string { return fmt.Sprintf("%d.%d", v.Major, v.Minor) }

// CheckVersion refuses a module whose major version differs from the host's.
func CheckVersion(host, module Version) error {
	if host.Major != module.Major {
		return fmt.Errorf("the module speaks contract %s, and this app speaks %s", module, host)
	}
	return nil
}

// EncodeVersion converts v to the contract.
func EncodeVersion(v Version) *pb.Version { return &pb.Version{Major: v.Major, Minor: v.Minor} }

// DecodeVersion converts v from the contract; an absent version is 0.0.
func DecodeVersion(v *pb.Version) Version { return Version{v.GetMajor(), v.GetMinor()} }
