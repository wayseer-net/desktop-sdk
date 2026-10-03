package sdktest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
	"wayseer/pkg/sdk"
	"wayseer/pkg/sdk/manifest"
	"wayseer/pkg/sdk/model"
	"wayseer/pkg/sdk/module"
)

// checkActions requires a valid catalogue, and Do with a cancelled context to fail promptly.
// Only a cancelled Do is tried, so the check never acts on the source.
func (s *session) checkActions(world *model.Snapshot) error {
	a, ok := s.m.(sdk.Actor)
	if !ok {
		return fmt.Errorf("%w: not an Actor", ErrSkipped)
	}
	acts := a.Actions()
	if len(acts) == 0 { // an external module is an Actor whether or not its process offers any
		return fmt.Errorf("%w: offers no actions", ErrSkipped)
	}
	if err := sdk.ValidateActions(acts); err != nil {
		return err
	}
	if err := s.c.checkDeclared(acts); err != nil {
		return err
	}
	var errs []error
	for _, act := range acts {
		req, ok := sampleRequest(world, s.c.Name, act)
		if !ok {
			continue
		}
		errs = append(errs, cancelledQuery(func(ctx context.Context) error {
			_, err := a.Do(ctx, req)
			return err
		}))
	}
	return errors.Join(errs...)
}

// sampleRequest asks for act on the first entity of the instance that offers it, with each
// parameter at its default or its lowest value.
func sampleRequest(world *model.Snapshot, inst sdk.ModuleID, act sdk.Action) (sdk.ActionRequest, bool) {
	for ref := range world.BySource(inst) {
		if e, ok := world.Entity(ref); ok && slices.Contains(act.Kinds, e.Kind) {
			return sdk.ActionRequest{Instance: inst, Action: act.ID, Entity: ref, Params: sampleParams(act)}, true
		}
	}
	return sdk.ActionRequest{}, false
}

func sampleParams(act sdk.Action) map[string]string {
	out := map[string]string{}
	for _, p := range act.Params {
		switch {
		case p.Default != "":
			out[p.Name] = p.Default
		case p.Type == sdk.ParamInt:
			out[p.Name] = strconv.FormatInt(p.Min, 10)
		case p.Type == sdk.ParamDuration:
			out[p.Name] = time.Duration(p.Min).String()
		case p.Type == sdk.ParamChoice:
			out[p.Name] = p.Choices[0]
		}
	}
	return out
}

// checkDeclared fails on any of acts the case's manifest, if it names one, doesn't declare.
func (c Case) checkDeclared(acts []sdk.Action) error {
	if c.Manifest == "" {
		return nil
	}
	b, err := os.ReadFile(c.Manifest)
	if err != nil {
		return fmt.Errorf("reading the manifest: %w", err)
	}
	_, m, err := manifest.Fill(b, manifest.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH, SHA256: strings.Repeat("0", 64)})
	if err != nil {
		return fmt.Errorf("%s: %w", c.Manifest, err)
	}
	if _, left := module.KeepDeclared(acts, module.DeclaredIn(m)); len(left) > 0 {
		return fmt.Errorf("the manifest doesn't declare %s, so Wayseer won't offer it", strings.Join(left, ", "))
	}
	return nil
}
