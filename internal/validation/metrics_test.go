package validation

import (
	"testing"
)

func TestRecall(t *testing.T) {
	tests := []struct {
		name     string
		expected []string
		actual   []string
		want     float64
	}{
		{"perfect", []string{"a", "b", "c"}, []string{"a", "b", "c"}, 1.0},
		{"partial", []string{"a", "b", "c"}, []string{"a", "b"}, 2.0 / 3.0},
		{"none", []string{"a", "b", "c"}, []string{"d", "e"}, 0.0},
		{"empty expected", []string{}, []string{"a", "b"}, 1.0},
		{"empty actual", []string{"a", "b"}, []string{}, 0.0},
		{"both empty", []string{}, []string{}, 1.0},
		{"extras ok", []string{"a", "b"}, []string{"a", "b", "c", "d"}, 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Recall(tt.expected, tt.actual)
			if !floatClose(got, tt.want) {
				t.Errorf("Recall() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrecision(t *testing.T) {
	tests := []struct {
		name     string
		expected []string
		actual   []string
		want     float64
	}{
		{"perfect", []string{"a", "b", "c"}, []string{"a", "b", "c"}, 1.0},
		{"with extras", []string{"a", "b"}, []string{"a", "b", "c", "d"}, 0.5},
		{"none match", []string{"a", "b"}, []string{"c", "d"}, 0.0},
		{"empty actual", []string{"a"}, []string{}, 1.0},
		{"empty expected", []string{}, []string{"a", "b"}, 0.0},
		{"both empty", []string{}, []string{}, 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Precision(tt.expected, tt.actual)
			if !floatClose(got, tt.want) {
				t.Errorf("Precision() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMissing(t *testing.T) {
	got := Missing([]string{"a", "b", "c"}, []string{"b", "d"})
	assertStringSlice(t, got, []string{"a", "c"})
}

func TestExtra(t *testing.T) {
	got := Extra([]string{"a", "b"}, []string{"a", "b", "c", "d"})
	assertStringSlice(t, got, []string{"c", "d"})
}

func TestContainsAll(t *testing.T) {
	if !ContainsAll([]string{"a", "b"}, []string{"a", "b", "c"}) {
		t.Error("expected true")
	}
	if ContainsAll([]string{"a", "b", "x"}, []string{"a", "b", "c"}) {
		t.Error("expected false")
	}
	if !ContainsAll([]string{}, []string{"a"}) {
		t.Error("empty required should be true")
	}
}

func TestContainsAny(t *testing.T) {
	if !ContainsAny([]string{"a", "x"}, []string{"a", "b", "c"}) {
		t.Error("expected true")
	}
	if ContainsAny([]string{"x", "y"}, []string{"a", "b", "c"}) {
		t.Error("expected false")
	}
	if ContainsAny([]string{}, []string{"a"}) {
		t.Error("empty items should be false")
	}
}

func floatClose(a, b float64) bool {
	const eps = 1e-9
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < eps
}

func assertStringSlice(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("got %v, want %v", got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
