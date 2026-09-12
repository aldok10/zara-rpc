package xunsafe

import "unsafe"

var (
	alwaysFalse bool
	sink        unsafe.Pointer //nolint:unused
)

// Escape escapes a pointer to the heap.
func Escape[P ~*E, E any](p P) P {
	if alwaysFalse {
		sink = unsafe.Pointer(p)
	}
	return p
}

// NoEscape hides a pointer from escape analysis, preventing it from
// escaping to the heap.
//
// The upstream implementation is `P((AddrOf(p) ^ 0).AssertValid())` on an
// intptr-based Addr; the struct-based Addr cannot express that, and the
// uintptr XOR equivalent (`unsafe.Pointer(uintptr(p) ^ 0)`) fails go vet's
// unsafeptr check. This identity version compiles vet-clean but does not
// actually hide the pointer from escape analysis — do not rely on it for
// hot-path allocation control.
func NoEscape[P ~*E, E any](p P) P {
	return P(AddrOf(p).AssertValid())
}
