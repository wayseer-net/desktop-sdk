package module

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCheckFillsDefaultsAndKeepsGivenValues(t *testing.T) {
	got, err := silence.Check(map[string]string{"reason": "deploy"})
	if err != nil || got["for"] != "1h" || got["reason"] != "deploy" {
		t.Errorf("Check = %v, %v", got, err)
	}
}

func TestCheckRefusesBadParameters(t *testing.T) {
	cases := []struct {
		a      Action
		params map[string]string
		want   error
		says   string
	}{
		{scale, map[string]string{}, ErrParamMissing, "replicas"},
		{scale, map[string]string{"replicas": "3", "force": "yes"}, ErrParamUnknown, "force"},
		{scale, map[string]string{"replicas": "21"}, ErrParamBounds, "0 to 20"},
		{scale, map[string]string{"replicas": "-1"}, ErrParamBounds, "0 to 20"},
		{scale, map[string]string{"replicas": "three"}, ErrParamValue, "whole number"},
		{silence, map[string]string{"for": "30s", "reason": "deploy"}, ErrParamBounds, "1m0s to 24h0m0s"},
		{silence, map[string]string{"for": "soon", "reason": "deploy"}, ErrParamValue, "duration"},
		{silence, map[string]string{"reason": "boredom"}, ErrParamValue, "deploy, incident"},
	}
	for _, c := range cases {
		_, err := c.a.Check(c.params)
		if !errors.Is(err, c.want) || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s %v: err = %v, want %v saying %q", c.a.ID, c.params, err, c.want, c.says)
		}
	}
}

func TestCheckAcceptsTheBounds(t *testing.T) {
	for _, v := range []string{"0", "20"} {
		if _, err := scale.Check(map[string]string{"replicas": v}); err != nil {
			t.Errorf("replicas %s: %v", v, err)
		}
	}
}

func TestRequestReadsCheckedValues(t *testing.T) {
	r := ActionRequest{Params: map[string]string{"replicas": "3", "for": "90m", "reason": "deploy"}}
	if r.Int("replicas") != 3 || r.Duration("for") != 90*time.Minute || r.Choice("reason") != "deploy" {
		t.Errorf("read %d %v %q", r.Int("replicas"), r.Duration("for"), r.Choice("reason"))
	}
}

func TestValidateActionsRefusesABadCatalogue(t *testing.T) {
	cases := map[string][]Action{
		"bad id":          {{ID: "Re start", Title: "R", Changes: "c", Kinds: restart.Kinds}},
		"duplicate id":    {restart, restart},
		"no title":        {{ID: "r", Changes: "c", Kinds: restart.Kinds}},
		"no changes":      {{ID: "r", Title: "R", Kinds: restart.Kinds}},
		"no kinds":        {{ID: "r", Title: "R", Changes: "c"}},
		"bounds reversed": {{ID: "r", Title: "R", Changes: "c", Kinds: restart.Kinds, Params: []Param{IntParam("n", "N", 5, 1)}}},
		"no choices":      {{ID: "r", Title: "R", Changes: "c", Kinds: restart.Kinds, Params: []Param{ChoiceParam("c", "C")}}},
		"bad default":     {{ID: "r", Title: "R", Changes: "c", Kinds: restart.Kinds, Params: []Param{IntParam("n", "N", 0, 3).WithDefault("9")}}},
		"repeated param":  {{ID: "r", Title: "R", Changes: "c", Kinds: restart.Kinds, Params: []Param{IntParam("n", "N", 0, 3), IntParam("n", "N", 0, 3)}}},
	}
	for name, acts := range cases {
		if err := ValidateActions(acts); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := ValidateActions([]Action{scale, silence, restart}); err != nil {
		t.Errorf("sound catalogue: %v", err)
	}
}
