package suite

import "testing"

func TestPercentileNearestRank(t *testing.T) {
	values := make([]float64, 100)
	for i := range 100 {
		values[i] = float64(i + 1)
	}
	if got := Percentile(values, 50); got != 50 {
		t.Fatalf("P50=%v want 50", got)
	}
	if got := Percentile(values, 90); got != 90 {
		t.Fatalf("P90=%v want 90", got)
	}
	if got := Percentile(values, 99); got != 99 {
		t.Fatalf("P99=%v want 99", got)
	}
	if got := Percentile(nil, 95); got != 0 {
		t.Fatalf("empty=%v want 0", got)
	}
	if got := Percentile([]float64{5}, 99); got != 5 {
		t.Fatalf("single=%v want 5", got)
	}
}

func TestCV(t *testing.T) {
	if got := CV([]float64{7, 7, 7}); got != 0 {
		t.Fatalf("constant CV=%v want 0", got)
	}
	// mean=2, std=1 → CV=0.5
	if got := CV([]float64{1, 3}); got < 0.49 || got > 0.51 {
		t.Fatalf("CV=%v want ≈0.5", got)
	}
}
