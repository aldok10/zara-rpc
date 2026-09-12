package xunsafe

import (
	"unsafe"

	"github.com/aldok10/zara-rpc/internal/xunsafe/layout"
)

// VLA is a mechanism for accessing a variable-length array that follows
// some struct.
type VLA[T any] [0]T

// Beyond obtains the VLA past the end of p.
// Address calculation assumes that p is well-aligned.
func Beyond[T, Header any](p *Header) *VLA[T] {
	align := layout.Align[T]()
	// Upstream converts the Addr to a different element type directly; the
	// struct-based Addr cannot do that, so convert through unsafe.Pointer
	// (a plain pointer conversion, no uintptr round-trip).
	return (*VLA[T])(unsafe.Pointer(AddrOf(p).Add(1).RoundUpTo(align).AssertValid()))
}

func (a *VLA[T]) Get(n int) *T {
	return Add(Cast[T](a), n)
}

func (a *VLA[T]) ByteGet(n int) *T {
	return ByteAdd[T](a, n)
}

func (a *VLA[T]) Slice(n int) []T {
	return unsafe.Slice(a.Get(0), n)
}
