package xsync

import "testing"

func TestSetStoreLoad(t *testing.T) {
	var s Set[string]
	s.Store("a")
	if !s.Load("a") {
		t.Fatal("Load(a) = false, want true")
	}
	if s.Load("b") {
		t.Fatal("Load(b) = true, want false")
	}
}

func TestSetAll(t *testing.T) {
	var s Set[string]
	s.Store("a")
	s.Store("b")
	got := map[string]bool{}
	for k := range s.All() {
		got[k] = true
	}
	if len(got) != 2 || !got["a"] || !got["b"] {
		t.Fatalf("All() = %v, want {a b}", got)
	}
}