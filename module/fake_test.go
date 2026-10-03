package module

import (
	"context"
	"time"
	"wayseer/pkg/sdk/data"
	"wayseer/pkg/sdk/model"
)

// fake is a Module that does nothing, for registry and capability tests.
type fake struct{}

func (*fake) Info() Info                              { return Info{Kind: "fake", Version: "1"} }
func (*fake) Configure(context.Context, Config) error { return nil }
func (*fake) Run(ctx context.Context, _ Sink) error   { <-ctx.Done(); return nil }
func (*fake) Health() data.Health                     { return data.Health{} }

func testRegistry(f func() Module) *Registry {
	var r Registry
	r.Register("fake", f)
	return &r
}

var scale = Action{
	ID: "scale", Title: "Scale", Changes: "Sets how many replicas run",
	Kinds:  []model.Kind{model.KindService},
	Params: []Param{IntParam("replicas", "Replicas", 0, 20)},
}

var silence = Action{
	ID: "silence", Title: "Silence", Changes: "Mutes the alert's notifications",
	Kinds: []model.Kind{model.KindService},
	Params: []Param{
		DurationParam("for", "For", time.Minute, 24*time.Hour).WithDefault("1h"),
		ChoiceParam("reason", "Reason", "deploy", "incident"),
	},
}

var restart = Action{ID: "restart", Title: "Restart", Changes: "Replaces every pod", Kinds: []model.Kind{model.KindHost}}
