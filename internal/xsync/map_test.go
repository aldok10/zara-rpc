package xsync

import "testing"

func TestMapStoreLoad(t *testing.T) {
	var m Map[string, int]
	m.Store("a", 1)
	if v, ok := m.Load("a"); !ok || v != 1 {
		t.Fatalf("Load(a) = %d, %v; want 1, true", v, ok)
	}
	if _, ok := m.Load("b"); ok {
		t.Fatal("Load(b) = ok, want not ok")
	}
}

func TestMapLoadOrStore(t *testing.T) {
	var m Map[string, int]
	v, loaded := m.LoadOrStore("a", func() int { return 42 })
	if loaded || v != 42 {
		t.Fatalf("LoadOrStore(a) = %d, %v; want 42, false", v, loaded)
	}
	v, loaded = m.LoadOrStore("a", func() int { return 99 })
	if !loaded || v != 42 {
		t.Fatalf("LoadOrStore(a) second = %d, %v; want 42, true", v, loaded)
	}
}

func TestMapAll(t *testing.T) {
	var m Map[string, int]
	m.Store("a", 1)
	m.Store("b", 2)
	got := map[string]int{}
	for k, v := range m.All() {
		got[k] = v
	}
	if len(got) != 2 || got["a"] != 1 || got["b"] != 2 {
		t.Fatalf("All() = %v, want {a:1 b:2}", got)
	}
}