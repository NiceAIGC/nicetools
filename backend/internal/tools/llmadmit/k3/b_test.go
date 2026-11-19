package k3

import (
	"reflect"
	"testing"
)

func TestDownsamplePreservesRequestOrderAndEndpoints(t *testing.T) {
	got := bDownsample([]float64{90, 80, 70, 60, 50, 40, 30, 20}, 4)
	want := []float64{90, 70, 50, 20}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("downsample=%v, want %v", got, want)
	}
}

func TestPadMP4KeepsExactSize(t *testing.T) {
	size := 2 << 20
	padded := PadMP4(redMP4, size)
	if len(padded) != size {
		t.Fatalf("padded=%d want %d", len(padded), size)
	}
	if string(padded[:len(redMP4)]) != string(redMP4) {
		t.Fatal("original MP4 prefix changed")
	}
}
