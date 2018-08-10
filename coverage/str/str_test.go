package str

import (
	"testing"
)

func TestPercentStr(t *testing.T) {
	f := float32(0.67895)
	expected := "67.9%"
	if PercentStr(f) != expected {
		t.Errorf("expected=%s, actual=%f", expected, f)
	}
}
