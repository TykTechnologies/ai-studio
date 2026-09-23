package stats

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestPercentileInterpolates(t *testing.T) {
	xs := []float64{4, 1, 3, 2, 5}
	cases := map[float64]float64{0: 1, 50: 3, 100: 5, 25: 2, 90: 4.6}
	for p, want := range cases {
		if got := Percentile(xs, p); math.Abs(got-want) > 1e-9 {
			t.Errorf("p%v = %v, want %v", p, got, want)
		}
	}
	if xs[0] != 4 {
		t.Fatal("Percentile must not reorder its input")
	}
}

func TestDiffOfPercentilesCoversTrueShift(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	var a, b []float64
	for i := 0; i < 4000; i++ {
		a = append(a, 100+10*r.NormFloat64())
		b = append(b, 103+10*r.NormFloat64())
	}
	ci := DiffOfPercentiles(a, b, 50, 0.95, 7)
	if !ci.Contains(3) || ci.Contains(0) {
		t.Fatalf("CI %+v should contain the true 3ms shift and exclude 0", ci)
	}
}

func TestDiffOfPercentilesAAContainsZero(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	var a, b []float64
	for i := 0; i < 3000; i++ {
		a = append(a, 50+5*r.ExpFloat64())
		b = append(b, 50+5*r.ExpFloat64())
	}
	if ci := DiffOfPercentiles(a, b, 50, 0.95, 7); !ci.Contains(0) {
		t.Fatalf("A/A CI %+v should contain 0", ci)
	}
}

func TestMedianOfPaired(t *testing.T) {
	var d []float64
	for i := 0; i < 301; i++ {
		d = append(d, float64(i%7)-1) // median 2
	}
	ci := MedianOfPaired(d, 0.95, 1)
	if ci.Estimate != 2 || !ci.Contains(2) {
		t.Fatalf("%+v", ci)
	}
}

func TestSlope(t *testing.T) {
	x := []float64{0, 1, 2, 3}
	y := []float64{1, 3, 5, 7}
	if s := Slope(x, y); math.Abs(s-2) > 1e-9 {
		t.Fatal(s)
	}
}
