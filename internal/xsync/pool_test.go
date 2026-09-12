package xsync

import (
	"bytes"
	"testing"

	"github.com/aldok10/zara-rpc/internal/xunsafe"
)

func TestBufferRoundTrip(t *testing.T) {
	p := NewBuffer(1024)
	b := p.Get()
	b.WriteString("hello")
	if got := b.String(); got != "hello" {
		t.Fatalf("buffer content = %q, want %q", got, "hello")
	}
	p.Put(b)
	// A fresh Get must return a reset buffer.
	b2 := p.Get()
	if b2.Len() != 0 {
		t.Fatalf("reused buffer not reset: len = %d", b2.Len())
	}
	p.Put(b2)
}

func TestBufferDropsOversized(t *testing.T) {
	p := NewBuffer(64)
	b := p.Get()
	b.Grow(4096)
	p.Put(b)
	// The oversized buffer must not be retained: a subsequent Get must not
	// carry its capacity.
	b2 := p.Get()
	if b2.Cap() > 64 {
		t.Fatalf("oversized buffer retained: cap = %d", b2.Cap())
	}
	p.Put(b2)
}

func TestBytesGetNPut(t *testing.T) {
	p := NewBytes(1024)
	b := p.GetN(10)
	if len(*b) != 10 {
		t.Fatalf("GetN(10) len = %d", len(*b))
	}
	for i := range *b {
		(*b)[i] = byte(i)
	}
	p.Put(b)
	b2 := p.GetN(10)
	if len(*b2) != 10 {
		t.Fatalf("GetN(10) after Put len = %d", len(*b2))
	}
	// The pooled slice must be reused (same backing array), not reallocated.
	if &(*b2)[0] != &(*b)[0] {
		t.Fatal("pooled slice was not reused")
	}
	p.Put(b2)
}

func TestBytesDropsOversized(t *testing.T) {
	p := NewBytes(64)
	b := p.GetN(4096)
	p.Put(b)
	b2 := p.GetN(1)
	if cap(*b2) > 64 {
		t.Fatalf("oversized slice retained: cap = %d", cap(*b2))
	}
	p.Put(b2)
}

func TestBytesGetNGrows(t *testing.T) {
	p := NewBytes(1024)
	b := p.GetN(10)
	p.Put(b)
	// Requesting more than the pooled capacity must allocate a fresh slice.
	b2 := p.GetN(2048)
	if len(*b2) != 2048 {
		t.Fatalf("GetN(2048) len = %d", len(*b2))
	}
	p.Put(b2)
}

// TestPooledTypesBoxFree locks the boxing-tax invariant: Pool stores *T,
// and a pointer boxes into an interface for free. A raw sync.Pool of []byte
// would allocate the slice header on every Put (the boxing tax).
func TestPooledTypesBoxFree(t *testing.T) {
	if !xunsafe.IsDirect[*[]byte]() {
		t.Fatal("IsDirect[*[]byte]() = false, want true: Bytes pool would allocate on Put")
	}
	if !xunsafe.IsDirect[*bytes.Buffer]() {
		t.Fatal("IsDirect[*bytes.Buffer]() = false, want true: Buffer pool would allocate on Put")
	}
}

func BenchmarkBytesGetNPut(b *testing.B) {
	p := NewBytes(1 << 20)
	b.ReportAllocs()
	for b.Loop() {
		buf := p.GetN(1024)
		for j := range *buf {
			(*buf)[j] = byte(j)
		}
		p.Put(buf)
	}
}

func BenchmarkBufferGetPut(b *testing.B) {
	p := NewBuffer(1 << 20)
	b.ReportAllocs()
	for b.Loop() {
		buf := p.Get()
		buf.WriteString("hello world")
		p.Put(buf)
	}
}