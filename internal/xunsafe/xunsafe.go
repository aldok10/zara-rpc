package xunsafe

import (
	"sync"
	"unsafe"

	"github.com/aldok10/zara-rpc/internal/xunsafe/layout"
)

// NoCopy is a type that go vet will complain about having been moved.
//
// It does so by implementing [sync.Locker].
type NoCopy [0]sync.Mutex

// Int is any integer type.
type Int = layout.Int

// BitCast performs an unsafe bitcast from one type to another.
func BitCast[To, From any](v From) To {
	return *(*To)(unsafe.Pointer(&v))
}

// Ping reminds the processor that *p should be loaded into the data cache.
func Ping[P ~*E, E any](p P) {
	_ = ByteLoad[byte](NoEscape(p), 0)
}
