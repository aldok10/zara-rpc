// Reusable byte buffers for framework-owned I/O.
//
// The framework hands most per-request data to user code (path params,
// query values, request bodies, messages, responses) — that data must NOT
// be pooled, because handlers may retain it beyond the request. These pools
// are only for buffers the framework owns outright: codec output that is
// written to the wire and dropped, and read buffers that are decoded and
// dropped.
//
// Both pool types enforce a size cap: buffers larger than the cap are
// dropped on Put, so a single oversized response cannot pin memory forever.
package xsync

import "bytes"

// Buffer is a pool of *bytes.Buffer for read-into patterns
// (io.Reader -> buffer -> decode).
type Buffer struct {
	pool Pool[bytes.Buffer]
	max  int
}

// NewBuffer creates a Buffer pool that drops buffers larger than max bytes.
func NewBuffer(max int) *Buffer {
	return &Buffer{
		max: max,
		pool: Pool[bytes.Buffer]{
			New: func() *bytes.Buffer { return new(bytes.Buffer) },
		},
	}
}

// Get returns a reset buffer from the pool.
func (p *Buffer) Get() *bytes.Buffer {
	return p.pool.Get()
}

// Put returns a buffer to the pool. Buffers larger than max are replaced
// with a fresh empty buffer so oversized memory is not retained.
func (p *Buffer) Put(b *bytes.Buffer) {
	b.Reset()
	if b.Cap() > p.max {
		*b = bytes.Buffer{}
	}
	p.pool.Put(b)
}

// Bytes is a pool of []byte for codec output that is written and dropped.
// It stores *[]byte so Get/Put are allocation-free (see package doc).
type Bytes struct {
	pool Pool[[]byte]
	max  int
}

// NewBytes creates a Bytes pool that drops slices larger than max bytes.
func NewBytes(max int) *Bytes {
	return &Bytes{
		max: max,
		pool: Pool[[]byte]{
			New: func() *[]byte { return new([]byte) },
		},
	}
}

// GetN returns a pointer to a slice of length n, growing the pooled slice
// when needed. The caller must overwrite every byte it reads and must
// return the pointer with Put.
func (p *Bytes) GetN(n int) *[]byte {
	b := p.pool.Get()
	if cap(*b) < n {
		*b = make([]byte, n)
	} else {
		*b = (*b)[:n]
	}
	return b
}

// Put returns a slice pointer to the pool. Slices larger than max are
// dropped (the pointer is nil'd so the backing array can be collected).
// The caller must not touch the slice after Put.
func (p *Bytes) Put(b *[]byte) {
	if cap(*b) > p.max {
		*b = nil
		return
	}
	*b = (*b)[:0]
	p.pool.Put(b)
}