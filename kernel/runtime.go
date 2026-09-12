package kernel

import (
	"reflect"

	"github.com/aldok10/zara-rpc/encoding"
	"github.com/aldok10/zara-rpc/runtime"
)

// elemType returns the element type of T, dereferencing pointers. Generated
// Operations use pointer stream types (e.g. ClientStream[*User]), so the
// decode target must be the value type (User), not the pointer.
func elemType[T any]() reflect.Type {
	t := reflect.TypeOf((*T)(nil)).Elem()
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

// StreamType returns the operation's streaming mode.
func (e *Operation) StreamType() runtime.StreamType {
	return e.streamType
}

// Codec returns the operation's codec.
func (e *Operation) Codec() encoding.Codec {
	return e.codec
}

// ResponseType returns the response message type.
func (e *Operation) ResponseType() reflect.Type {
	return e.resType
}

// RequestType returns the request message type.
func (e *Operation) RequestType() reflect.Type {
	return e.reqType
}
