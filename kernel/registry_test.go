package kernel

import (
	"testing"
)

func TestServiceBuilder(t *testing.T) {
	var s Service
	built := (*ServiceBuilder)(&s).
		SetName("test.v1.TestService").
		Build()

	if built == nil {
		t.Fatal("Build returned nil")
	}
	if built.Name != "test.v1.TestService" {
		t.Errorf("Name = %q, want test.v1.TestService", built.Name)
	}
}

func BenchmarkServiceBuilder(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var s Service
		(*ServiceBuilder)(&s).SetName("test.v1.TestService").Build()
	}
}