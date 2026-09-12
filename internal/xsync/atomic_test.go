package xsync

import "testing"

func TestAtomicFloat64(t *testing.T) {
	var x AtomicFloat64
	x.Store(1.5)
	if got := x.Load(); got != 1.5 {
		t.Fatalf("Load() = %v, want 1.5", got)
	}
	if got := x.Add(2.25); got != 3.75 {
		t.Fatalf("Add(2.25) = %v, want 3.75", got)
	}
	if got := x.Swap(10); got != 3.75 {
		t.Fatalf("Swap(10) = %v, want 3.75", got)
	}
	if !x.BitwiseCompareAndSwap(10, 20) {
		t.Fatal("BitwiseCompareAndSwap(10, 20) = false, want true")
	}
	if got := x.Load(); got != 20 {
		t.Fatalf("Load() after CAS = %v, want 20", got)
	}
}