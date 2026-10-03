package model

import (
	"errors"
	"testing"
)

func matchWorld(t *testing.T, es ...Entity) *Matcher {
	t.Helper()
	snap, err := identityStore().Apply(&ChangeSet{Upserts: es})
	if err != nil {
		t.Fatal(err)
	}
	return NewMatcher(snap, DefaultIdentityRules())
}

func service(src, native, name string) Entity {
	return Entity{Ref: mustRef(src, KindService, native), Kind: KindService, Name: name, Source: ModuleID(src)}
}

func TestMatchFindsAnEntityByItsName(t *testing.T) {
	checkout := service("k8s", "shop/checkout", "checkout")
	m := matchWorld(t, checkout, service("k8s", "shop/cart", "cart"))
	if r, err := m.Match(KindService, "checkout"); err != nil || r != checkout.Ref {
		t.Errorf("checkout matches %s, %v", r, err)
	}
	if _, err := m.Match(KindService, "payments"); !errors.Is(err, ErrNoMatch) {
		t.Errorf("payments matches with %v, want ErrNoMatch", err)
	}
	if _, err := m.Match(KindHost, "checkout"); !errors.Is(err, ErrNoMatch) {
		t.Errorf("a host named checkout matches with %v, want ErrNoMatch", err)
	}
}

func TestMatchFollowsTheIdentityRules(t *testing.T) {
	db := host("k8s", "db-07", "db-07.lan", map[string]Value{"ip": String("10.0.0.7")})
	m := matchWorld(t, db)
	for _, v := range []string{"DB-07:9100", "db-07.example.com", "10.0.0.7:5432"} {
		if r, err := m.Match(KindHost, v); err != nil || r != db.Ref {
			t.Errorf("%q matches %s, %v", v, r, err)
		}
	}
}

func TestMatchTakesOneOfEntitiesLinkedAsTheSame(t *testing.T) {
	a := host("prom", "db-07", "db-07", nil)
	b := host("k8s", "db-07", "db-07.lan", nil)
	m := matchWorld(t, b, a)
	if r, err := m.Match(KindHost, "db-07"); err != nil || r != min(a.Ref, b.Ref) {
		t.Errorf("db-07 matches %s, %v; want %s", r, err, min(a.Ref, b.Ref))
	}
}

func TestMatchRefusesANameOfUnlinkedEntities(t *testing.T) {
	m := matchWorld(t, service("k8s", "shop/checkout", "checkout"), service("k8s", "test/checkout", "checkout"))
	if _, err := m.Match(KindService, "checkout"); !errors.Is(err, ErrAmbiguous) {
		t.Errorf("checkout in two namespaces matches with %v, want ErrAmbiguous", err)
	}
	if _, err := m.Match(KindService, ""); !errors.Is(err, ErrNoMatch) {
		t.Errorf("an empty value matches with %v", err)
	}
}

func TestMatchFindsAnEntityByItsNativeID(t *testing.T) {
	shop, test := service("k8s", "shop/checkout", "checkout"), service("k8s", "test/checkout", "checkout")
	m := matchWorld(t, shop, test)
	if r, err := m.Match(KindService, "test/checkout"); err != nil || r != test.Ref {
		t.Errorf("test/checkout matches %s, %v; want %s", r, err, test.Ref)
	}
}

func TestMatchExceptLeavesOutWhatItIsTold(t *testing.T) {
	made := service("prom", "checkout", "checkout")
	found := service("k8s", "shop/checkout", "checkout")
	m := matchWorld(t, made, found)
	if _, err := m.Match(KindService, "checkout"); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("checkout matches with %v, want ErrAmbiguous", err)
	}
	notMade := func(r EntityRef) bool { return r == made.Ref }
	if r, err := m.MatchExcept(KindService, "checkout", notMade); err != nil || r != found.Ref {
		t.Errorf("checkout matches %s, %v; want %s", r, err, found.Ref)
	}
	if _, err := m.MatchExcept(KindService, "checkout", func(EntityRef) bool { return true }); !errors.Is(err, ErrNoMatch) {
		t.Errorf("with every entity left out, checkout matches with %v", err)
	}
}
