package wire_test

import (
	"fmt"
	"mindseye/pkg/sdk"
	"mindseye/pkg/sdk/wire"
)

// A module streams a change set in parts; the host joins them and checks the version first.
func Example() {
	if err := wire.CheckVersion(wire.Protocol, wire.Version{Major: 2}); err != nil {
		fmt.Println(err)
	}

	host := sdk.Entity{Ref: "files/host/web-1", Kind: sdk.KindHost, Name: "web-1", Source: "files"}
	cs := &sdk.ChangeSet{Upserts: []sdk.Entity{host}}
	var j wire.Joiner
	for _, part := range wire.Parts(true, wire.EncodeChangeSet(cs), wire.MaxPart) {
		if snapshot, whole, done := j.Add(part); done {
			fmt.Println(snapshot, wire.DecodeChangeSet(whole).Upserts[0].Name)
		}
	}
	// Output:
	// the module speaks contract 2.0, and this app speaks 1.4
	// true web-1
}
