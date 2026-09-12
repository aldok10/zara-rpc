package xunsafe

import (
	"bytes"
	"testing"

	"github.com/aldok10/zara-rpc/internal/xunsafe/layout"
)

func TestBitCast(t *testing.T) {
	// BitCast is only valid between same-size types: it reinterprets the
	// bits. 0x3FF0000000000000 is 1.0 in IEEE 754 double precision.
	if got := BitCast[float64](uint64(0x3FF0000000000000)); got != 1.0 {
		t.Fatalf("BitCast(uint64->float64) = %v, want 1.0", got)
	}
}

func TestSliceToStringStringToSlice(t *testing.T) {
	// The safety contract: the source must be immutable and must outlive
	// the conversion result. A package-level string satisfies both.
	b := StringToSlice[[]byte]("hello")
	if string(b) != "hello" {
		t.Fatalf("StringToSlice = %q, want hello", b)
	}
	if got := SliceToString(b); got != "hello" {
		t.Fatalf("SliceToString = %q, want hello", got)
	}
}

func TestIsDirect(t *testing.T) {
	// Pointer-shaped values box into an interface for free. This is the
	// invariant internal/xsync relies on: Pool stores *T, so Get/Put
	// never allocate. A raw sync.Pool of []byte would box the slice header
	// on every Put (the boxing tax).
	if !IsDirect[*int]() {
		t.Fatal("IsDirect[*int]() = false, want true")
	}
	if !IsDirect[*[]byte]() {
		t.Fatal("IsDirect[*[]byte]() = false, want true")
	}
	if !IsDirect[*bytes.Buffer]() {
		t.Fatal("IsDirect[*bytes.Buffer]() = false, want true")
	}
	// Non-pointer-shaped values box into an interface by allocating.
	if IsDirect[int]() {
		t.Fatal("IsDirect[int]() = true, want false")
	}
	if IsDirect[[]byte]() {
		t.Fatal("IsDirect[[]byte]() = true, want false")
	}
}

func TestAddrOfEndOf(t *testing.T) {
	var x int
	if AddrOf(&x).AssertValid() != &x {
		t.Fatal("AddrOf/AssertValid round-trip failed")
	}
	s := []int{1, 2, 3}
	end := EndOf(s)
	if end.Sub(AddrOf(&s[0])) != len(s) {
		t.Fatalf("EndOf offset = %d, want %d", end.Sub(AddrOf(&s[0])), len(s))
	}
}

func TestLayout(t *testing.T) {
	if layout.Size[int]() != 8 {
		t.Fatalf("Size[int]() = %d, want 8", layout.Size[int]())
	}
	if layout.RoundUp(5, 8) != 8 {
		t.Fatalf("RoundUp(5, 8) = %d, want 8", layout.RoundUp(5, 8))
	}
	if layout.Padding(5, 8) != 3 {
		t.Fatalf("Padding(5, 8) = %d, want 3", layout.Padding(5, 8))
	}
}