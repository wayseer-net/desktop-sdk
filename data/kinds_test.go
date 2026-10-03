package data

import (
	"context"
	"errors"
	"testing"
	"time"
	"wayseer/pkg/sdk/model"
)

func kindEntity(src model.ModuleID, kind model.Kind, native string) model.Entity {
	r, err := model.NewEntityRef(string(src), kind, native)
	if err != nil {
		panic(err)
	}
	return model.Entity{Ref: r, Kind: kind, Name: native, Source: src}
}

func TestAForeignKindRefusesTheWholeChangeSet(t *testing.T) {
	c, _, _ := newTestCoalescer(8, time.Now)
	c.Confine("ext", "acme")
	cs := &model.ChangeSet{Upserts: []model.Entity{
		kindEntity("ext", model.KindHost, "a"),
		kindEntity("ext", "k8s/deployment", "shop/web"),
	}}
	err := c.SubmitSnapshot(context.Background(), "ext", cs)
	if !errors.Is(err, ErrForeignKind) {
		t.Fatalf("err = %v, want ErrForeignKind", err)
	}
	if k, ok := errors.AsType[*ForeignKindError](err); !ok || k.Kind != "k8s/deployment" {
		t.Errorf("err = %#v, want it to name k8s/deployment", err)
	}
	snap, _ := c.Flush()
	if snap.Len() != 0 || c.Stats().Rejected != 1 {
		t.Errorf("entities = %d, rejected = %d; want nothing applied and one rejection", snap.Len(), c.Stats().Rejected)
	}
}

func TestCoreKindsAndTheModulesOwnPass(t *testing.T) {
	c, _, _ := newTestCoalescer(8, time.Now)
	c.Confine("ext", "acme")
	cs := &model.ChangeSet{Upserts: []model.Entity{
		kindEntity("ext", model.KindHost, "a"),
		kindEntity("ext", "acme/rack", "r1"),
	}}
	mustSubmit(t, c.Submit(context.Background(), "ext", cs))
	if snap, _ := c.Flush(); snap.Len() != 2 {
		t.Errorf("entities = %d, want 2", snap.Len())
	}
}

func TestAnUnconfinedSourceSendsAnyKind(t *testing.T) {
	c, _, _ := newTestCoalescer(8, time.Now)
	c.Confine("ext", "acme")
	cs := &model.ChangeSet{Upserts: []model.Entity{kindEntity("k8s", "k8s/deployment", "shop/web")}}
	mustSubmit(t, c.Submit(context.Background(), "k8s", cs))
}

func TestAnUnknownNamespaceAllowsOnlyCoreKinds(t *testing.T) {
	c, _, _ := newTestCoalescer(8, time.Now)
	c.Confine("ext", "")
	cs := &model.ChangeSet{Upserts: []model.Entity{kindEntity("ext", "acme/rack", "r1")}}
	if err := c.Submit(context.Background(), "ext", cs); !errors.Is(err, ErrForeignKind) {
		t.Errorf("err = %v, want ErrForeignKind", err)
	}
}

func TestRemovingASourceFreesItsKinds(t *testing.T) {
	c, _, _ := newTestCoalescer(8, time.Now)
	c.Confine("ext", "acme")
	mustSubmit(t, c.RemoveSource(context.Background(), "ext"))
	cs := &model.ChangeSet{Upserts: []model.Entity{kindEntity("ext", "k8s/deployment", "shop/web")}}
	mustSubmit(t, c.Submit(context.Background(), "ext", cs))
}
