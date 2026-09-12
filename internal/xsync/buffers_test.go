package xsync

import "testing"

func TestBufferPool(t *testing.T) {
	p := NewBuffer(1024)
	buf := p.Get()
	if buf == nil {
		t.Fatal("Get returned nil")
	}
	buf.WriteString("test")
	p.Put(buf)
	buf2 := p.Get()
	if buf2.Len() != 0 {
		t.Fatalf("pooled buffer not reset: len = %d", buf2.Len())
	}
}