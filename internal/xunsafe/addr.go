package xunsafe

import (
	"fmt"
	"unsafe"

	"github.com/aldok10/zara-rpc/internal/xunsafe/layout"
)

// Addr is a typed raw address.
//
// It stores a *T rather than a raw integer so that converting back to a
// pointer (AssertValid) is a plain field read.
type Addr[T any] struct {
	p *T
}

// AddrOf gets the address of a pointer.
func AddrOf[P ~*E, E any](p P) Addr[E] {
	return Addr[E]{p}
}

// EndOf calculates the one-past-the-end address of s without creating an
// intermediate one-past-the-end pointer.
func EndOf[S ~[]E, E any](s S) Addr[E] {
	return AddrOf(unsafe.SliceData(s)).Add(len(s))
}

// AssertValid asserts that this address is a valid pointer.
func (a Addr[T]) AssertValid() *T {
	return a.p
}

// Add adds the given offset to this address.
func (a Addr[T]) Add(n int) Addr[T] {
	return Addr[T]{(*T)(unsafe.Pointer(uintptr(unsafe.Pointer(a.p)) + uintptr(n*layout.Size[T]())))}
}

// ByteAdd adds the given unscaled offset to this address.
func (a Addr[T]) ByteAdd(n int) Addr[T] {
	return Addr[T]{(*T)(unsafe.Pointer(uintptr(unsafe.Pointer(a.p)) + uintptr(n)))}
}

// Sub computes the difference between two addresses, in units of T's size.
func (a Addr[T]) Sub(b Addr[T]) int {
	return int(uintptr(unsafe.Pointer(a.p))-uintptr(unsafe.Pointer(b.p))) / layout.Size[T]()
}

// Padding returns the number of bytes between this address and the next address
// aligned to the given alignment, which must be a power of two.
func (a Addr[T]) Padding(align int) int {
	return layout.Padding(int(uintptr(unsafe.Pointer(a.p))), align)
}

// RoundUpTo rounds this address upwards to align, which must be a power of two.
func (a Addr[T]) RoundUpTo(align int) Addr[T] {
	return Addr[T]{(*T)(unsafe.Pointer(uintptr(unsafe.Pointer(a.p)) + uintptr(layout.Padding(int(uintptr(unsafe.Pointer(a.p))), align))))}
}

// SignBit returns whether this address has its sign bit set.
//
// Pointers with the high bits set are never used by Go, so we can use this bit
// to store extra information.
func (a Addr[T]) SignBit() bool {
	return uintptr(unsafe.Pointer(a.p))>>(layout.Bits[uintptr]()-1) != 0
}

// ClearSignBit clears the sign bit of this address.
func (a Addr[T]) ClearSignBit() Addr[T] {
	return Addr[T]{(*T)(unsafe.Pointer(uintptr(unsafe.Pointer(a.p)) &^ (uintptr(1) << (layout.Bits[uintptr]() - 1))))}
}

// Format implements [fmt.Formatter].
func (a Addr[T]) Format(state fmt.State, verb rune) {
	if verb == 'v' {
		fmt.Fprintf(state, "%#x", uintptr(unsafe.Pointer(a.p)))
		return
	}

	fmt.Fprintf(state, fmt.FormatString(state, verb), uintptr(unsafe.Pointer(a.p)))
}
