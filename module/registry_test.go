package module

import (
	"context"
	"slices"
	"testing"

	"wayseer.dev/sdk/model"
)

func TestRegisterRejectsDuplicatesAndBadKinds(t *testing.T) {
	for _, kind := range []string{"fake", "", "Bad Kind"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Register(%q) did not panic", kind)
				}
			}()
			testRegistry(func() Module { return &fake{} }).Register(kind, func() Module { return &fake{} })
		}()
	}
}

func TestCapabilities(t *testing.T) {
	if got := Capabilities(&fake{}); len(got) != 0 {
		t.Errorf("plain module has %v", got)
	}
	if got := Capabilities(&searchFake{}); len(got) != 1 || got[0] != "search" {
		t.Errorf("searcher has %v", got)
	}
}

func TestTopIsListedOnlyForAModuleThatRanks(t *testing.T) {
	if got := Capabilities(&topFake{ranks: true}); !slices.Equal(got, []string{"top"}) {
		t.Errorf("a module that ranks has %v", got)
	}
	if got := Capabilities(&topFake{}); len(got) != 0 {
		t.Errorf("a module that could rank but does not has %v", got)
	}
}

type topFake struct {
	fake
	ranks bool
}

func (f *topFake) RanksTop() bool { return f.ranks }

type searchFake struct{ fake }

func (searchFake) Search(context.Context, string, int) ([]model.EntityRef, error) { return nil, nil }
