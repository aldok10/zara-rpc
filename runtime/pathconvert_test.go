package runtime

import (
	"testing"
)

func TestPathConverters(t *testing.T) {
	if s, err := PathString("hello"); err != nil || s != "hello" {
		t.Errorf("PathString = (%q, %v), want (hello, nil)", s, err)
	}

	if b, err := PathBool("true"); err != nil || !b {
		t.Errorf("PathBool = (%v, %v), want (true, nil)", b, err)
	}
	if _, err := PathBool("invalid"); err == nil {
		t.Error("PathBool(invalid) expected error")
	}

	if i, err := PathInt32("42"); err != nil || i != 42 {
		t.Errorf("PathInt32 = (%d, %v), want (42, nil)", i, err)
	}
	if _, err := PathInt32("99999999999999"); err == nil {
		t.Error("PathInt32 overflow expected error")
	}

	if i, err := PathInt64("100"); err != nil || i != 100 {
		t.Errorf("PathInt64 = (%d, %v), want (100, nil)", i, err)
	}

	if u, err := PathUint32("10"); err != nil || u != 10 {
		t.Errorf("PathUint32 = (%d, %v), want (10, nil)", u, err)
	}

	if u, err := PathUint64("200"); err != nil || u != 200 {
		t.Errorf("PathUint64 = (%d, %v), want (200, nil)", u, err)
	}

	if f, err := PathFloat32("3.14"); err != nil || f != 3.14 {
		t.Errorf("PathFloat32 = (%v, %v), want (3.14, nil)", f, err)
	}

	if f, err := PathFloat64("2.71828"); err != nil || f != 2.71828 {
		t.Errorf("PathFloat64 = (%v, %v), want (2.71828, nil)", f, err)
	}

	if bytes, err := PathBytes("aGVsbG8="); err != nil || string(bytes) != "hello" {
		t.Errorf("PathBytes = (%q, %v), want (hello, nil)", string(bytes), err)
	}
}