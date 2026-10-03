package module

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"wayseer.dev/sdk/manifest"
	"wayseer.dev/sdk/model"
)

// Actor is a module whose entities offer actions the owner may run (ADR-0008). The host runs
// one only if the config allows it, the entity's kind offers it and its parameters check.
type Actor interface {
	// Actions lists every action the module offers, each on the kinds it names.
	Actions() []Action
	// Do runs a request the host has checked; it must stop when ctx ends.
	Do(ctx context.Context, req ActionRequest) (ActionResult, error)
}

// Action is one thing a module can do to an entity, such as restarting it.
type Action struct {
	ID      string       // such as "restart": lower-case letters, digits and dashes
	Title   string       // such as "Restart"
	Changes string       // one line on what changes, shown before the owner confirms
	Kinds   []model.Kind // the kinds of entity it applies to
	Params  []Param
}

// ParamType is how a parameter's value is read.
type ParamType uint8

// Parameter types.
const (
	ParamInt      ParamType = iota + 1 // a whole number from Min to Max
	ParamDuration                      // a duration from Min to Max nanoseconds, as "90m"
	ParamChoice                        // one of Choices
)

// Param is one typed, bounded parameter of an action; every one is needed, or its Default used.
type Param struct {
	Name     string
	Title    string
	Type     ParamType
	Min, Max int64
	Choices  []string
	Default  string // empty if the value must be given
}

// IntParam is a whole number from lo to hi.
func IntParam(name, title string, lo, hi int64) Param {
	return Param{Name: name, Title: title, Type: ParamInt, Min: lo, Max: hi}
}

// DurationParam is a duration from lo to hi.
func DurationParam(name, title string, lo, hi time.Duration) Param {
	return Param{Name: name, Title: title, Type: ParamDuration, Min: int64(lo), Max: int64(hi)}
}

// ChoiceParam is one of choices.
func ChoiceParam(name, title string, choices ...string) Param {
	return Param{Name: name, Title: title, Type: ParamChoice, Choices: choices}
}

// WithDefault is p with the value used when none is given.
func (p Param) WithDefault(v string) Param { p.Default = v; return p }

// Why a parameter is refused; a *ParamError wraps one.
var (
	ErrParamUnknown = errors.New("no such parameter")
	ErrParamMissing = errors.New("missing")
	ErrParamValue   = errors.New("not a valid value")
	ErrParamBounds  = errors.New("out of bounds")
)

// ParamError says which parameter of which action was refused, and why.
type ParamError struct {
	Action, Param, Detail string
	Err                   error
}

func (e *ParamError) Error() string {
	return fmt.Sprintf("%s: %s: %s", e.Action, e.Param, e.Detail)
}

func (e *ParamError) Unwrap() error { return e.Err }

// Check returns params with defaults filled in, or the first parameter refused.
func (a *Action) Check(params map[string]string) (map[string]string, error) {
	for _, name := range sortedKeys(params) {
		if !slices.ContainsFunc(a.Params, func(p Param) bool { return p.Name == name }) {
			return nil, &ParamError{a.ID, name, "no such parameter", ErrParamUnknown}
		}
	}
	out := make(map[string]string, len(a.Params))
	for _, p := range a.Params {
		v, ok := params[p.Name]
		if !ok {
			v = p.Default
		}
		if v == "" {
			return nil, &ParamError{a.ID, p.Name, "missing", ErrParamMissing}
		}
		if detail, err := p.check(v); err != nil {
			return nil, &ParamError{a.ID, p.Name, detail, err}
		}
		out[p.Name] = v
	}
	return out, nil
}

// check says what is wrong with v, with ErrParamValue or ErrParamBounds.
func (p *Param) check(v string) (string, error) {
	switch p.Type {
	case ParamInt:
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Sprintf("%q is not a whole number", v), ErrParamValue
		}
		return p.bound(n, strconv.FormatInt(p.Min, 10), strconv.FormatInt(p.Max, 10))
	case ParamDuration:
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Sprintf("%q is not a duration, such as 90m", v), ErrParamValue
		}
		return p.bound(int64(d), time.Duration(p.Min).String(), time.Duration(p.Max).String())
	case ParamChoice:
		if !slices.Contains(p.Choices, v) {
			return fmt.Sprintf("%q is not one of %s", v, strings.Join(p.Choices, ", ")), ErrParamValue
		}
		return "", nil
	}
	return "unknown parameter type", ErrParamValue
}

func (p *Param) bound(n int64, lo, hi string) (string, error) {
	if n < p.Min || n > p.Max {
		return fmt.Sprintf("want %s to %s", lo, hi), ErrParamBounds
	}
	return "", nil
}

// ValidateActions checks a module's catalogue: IDs valid and unique, each action titled, saying
// what it changes, on some kind, and its parameters sound.
func ValidateActions(acts []Action) error {
	seen := map[string]bool{}
	for _, a := range acts {
		switch {
		case !manifest.ValidActionID(a.ID) || seen[a.ID]:
			return fmt.Errorf("action %q: the ID is invalid or repeated", a.ID)
		case a.Title == "" || a.Changes == "":
			return fmt.Errorf("action %s: needs a title and what it changes", a.ID)
		case len(a.Kinds) == 0:
			return fmt.Errorf("action %s: applies to no kind", a.ID)
		}
		seen[a.ID] = true
		if err := validateParams(a); err != nil {
			return err
		}
	}
	return nil
}

func validateParams(a Action) error {
	seen := map[string]bool{}
	for _, p := range a.Params {
		switch {
		case !manifest.ValidActionID(p.Name) || seen[p.Name] || p.Title == "":
			return fmt.Errorf("action %s: parameter %q: the name is invalid or repeated, or it has no title", a.ID, p.Name)
		case (p.Type == ParamInt || p.Type == ParamDuration) && p.Min > p.Max:
			return fmt.Errorf("action %s: parameter %s: its minimum is above its maximum", a.ID, p.Name)
		case p.Type == ParamChoice && len(p.Choices) == 0:
			return fmt.Errorf("action %s: parameter %s: no choices", a.ID, p.Name)
		case p.Type < ParamInt || p.Type > ParamChoice:
			return fmt.Errorf("action %s: parameter %s: unknown type", a.ID, p.Name)
		}
		seen[p.Name] = true
		if detail, err := p.check(p.Default); p.Default != "" && err != nil {
			return fmt.Errorf("action %s: parameter %s: default: %s", a.ID, p.Name, detail)
		}
	}
	return nil
}

// ActionRequest asks an instance to run an action on one of its entities.
type ActionRequest struct {
	Instance model.ModuleID
	Action   string
	Entity   model.EntityRef
	Params   map[string]string
}

// Int reads a checked whole-number parameter.
func (r ActionRequest) Int(name string) int64 {
	n, _ := strconv.ParseInt(r.Params[name], 10, 64)
	return n
}

// Duration reads a checked duration parameter.
func (r ActionRequest) Duration(name string) time.Duration {
	d, _ := time.ParseDuration(r.Params[name])
	return d
}

// Choice reads a checked choice parameter.
func (r ActionRequest) Choice(name string) string { return r.Params[name] }

// ActionResult is what an action did, in one line for the owner; it must hold no secret.
type ActionResult struct {
	Message string
}

// LateActor is an Actor whose catalogue is known only once it runs, as a Tier 2 module's is.
// The host hands it the actions the config allows, and it checks them with CheckAllowed then.
type LateActor interface {
	Actor
	AllowActions(ids []string)
}

// CheckAllowed checks a catalogue, and that it offers each action the config allows.
func CheckAllowed(cat []Action, allowed []string) error {
	if len(allowed) == 0 {
		return nil
	}
	if len(cat) == 0 {
		return errors.New("actions: the module offers no actions")
	}
	if err := ValidateActions(cat); err != nil {
		return err
	}
	ids := make([]string, len(cat))
	for i, act := range cat {
		ids[i] = act.ID
	}
	slices.Sort(ids)
	for _, id := range allowed {
		if !slices.Contains(ids, id) {
			return fmt.Errorf("unknown action %q (offers %s)", id, strings.Join(ids, ", "))
		}
	}
	return nil
}
