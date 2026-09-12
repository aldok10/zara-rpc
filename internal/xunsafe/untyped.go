package xunsafe

import "unsafe"

// ByteAdd adds the given offset to p, without scaling.
//
// It also throws in a cast for free.
//
// This is used in the VM to decode varints, but the VM unconditionally loads 8
// bytes for performance reasons. In practice, this is not a problem - if we
// end up loading data from another object, that data would be masked off, or
// handled by length checks later on.
//
// checkptr does not like us doing this. It could be fixed with a bounds check
// here, but that would add an unnecessary branch in the hottest part of the VM
// without changing observable behavior.
//
//go:nocheckptr
func ByteAdd[T any, P ~*E, E any, I Int](p P, n I) *T {
	return (*T)(unsafe.Add(unsafe.Pointer(p), n))
}

// ByteSub computes the difference between two pointers, without scaling.
func ByteSub[P1 ~*E1, P2 ~*E2, E1, E2 any](p1 P1, p2 P2) int {
	return int(uintptr(unsafe.Pointer(p1)) - uintptr(unsafe.Pointer(p2)))
}

// ByteLoad loads a value of the given type at the given byte offset.
func ByteLoad[T any, P ~*E, E any, I Int](p P, n I) T {
	return *ByteAdd[T](p, n)
}

// ByteStore stores a value of the given type at the given byte offset.
func ByteStore[T any, P ~*E, E any, I Int](p P, n I, v T) {
	*ByteAdd[T](p, n) = v
}
