package sdk_test

import (
	"context"
	"fmt"
	"os"
	"time"
	"wayseer/pkg/sdk"
	"wayseer/pkg/sdk/sdktest"
)

// hosts describes a fixed list of hosts named in its options.
type hosts struct {
	name sdk.ModuleID
	opts struct {
		Hosts []string `yaml:"hosts"`
	}
}

func (*hosts) Info() sdk.Info {
	return sdk.Info{Kind: "hosts", Version: "1", Description: "A fixed list of hosts"}
}

// Configure decodes the options; an unknown option is an error naming its line.
func (h *hosts) Configure(_ context.Context, cfg sdk.Config) error {
	h.name = cfg.Name
	return cfg.Decode(&h.opts)
}

// Run sends the hosts as a snapshot, then waits: the list never changes.
func (h *hosts) Run(ctx context.Context, sink sdk.Sink) error {
	cs := &sdk.ChangeSet{}
	for _, name := range h.opts.Hosts {
		ref, err := sdk.NewEntityRef(string(h.name), sdk.KindHost, name)
		if err != nil {
			return err
		}
		cs.Upserts = append(cs.Upserts, sdk.Entity{
			Ref: ref, Kind: sdk.KindHost, Name: name, Source: h.name, Seen: time.Now(),
			Status: sdk.Status{Level: sdk.StatusOK},
			Attrs:  map[string]sdk.Value{"cores": sdk.Number(8)},
		})
	}
	if err := sink.Snapshot(ctx, cs); err != nil {
		return err
	}
	<-ctx.Done()
	return nil
}

func (*hosts) Health() sdk.Health { return sdk.Health{} }

// A module is configured from its options, then runs, sending a snapshot to the sink. In the
// app, sdk.Register(Kind, factory) in the module's init makes the kind available to the config.
func Example() {
	cfg, err := sdktest.Config("lab", "hosts: [db-07, web-01]")
	if err != nil {
		panic(err)
	}
	m := &hosts{}
	if err := m.Configure(context.Background(), cfg); err != nil {
		panic(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var sink sdktest.Sink
	done := make(chan error)
	go func() { done <- m.Run(ctx, &sink) }()
	for len(sink.Sets()) == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	for _, e := range sink.Sets()[0].Upserts {
		fmt.Println(e.Ref, e.Status.Level)
	}
	// Output:
	// lab/host/db-07 ok
	// lab/host/web-01 ok
}

// A module's options embed SecretOptions to take a password or token from a file or an
// environment variable. The secret never prints.
func ExampleSecretOptions() {
	var opts struct {
		URL               string `yaml:"url"`
		sdk.SecretOptions `yaml:",inline"`
	}
	cfg, err := sdktest.Config("lab", "url: https://inventory.example\nsecret_env: INVENTORY_TOKEN")
	if err != nil {
		panic(err)
	}
	if err := cfg.Decode(&opts); err != nil {
		panic(err)
	}
	_, err = opts.Read()
	fmt.Println(err)

	if err := os.Setenv("INVENTORY_TOKEN", "hunter2"); err != nil {
		panic(err)
	}
	defer func() { _ = os.Unsetenv("INVENTORY_TOKEN") }()
	s, _ := opts.Read()
	fmt.Println(s, len(s.Reveal()))
	// Output:
	// secret_env: INVENTORY_TOKEN is not set
	// [redacted] 7
}
