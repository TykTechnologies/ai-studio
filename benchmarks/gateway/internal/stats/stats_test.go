package stats

import (
	"math"
	"math/rand/v2"
	"sort"
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

// percentileInPlace must return exactly what sorting and PercentileSorted
// return, ties, NaNs and infinities included: the bootstrap CIs in every
// report depend on it.
func TestPercentileInPlaceMatchesSort(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	same := func(a, b float64) bool {
		return math.Float64bits(a) == math.Float64bits(b) || (math.IsNaN(a) && math.IsNaN(b))
	}
	for iter := 0; iter < 1000; iter++ {
		n := 1 + r.IntN(300)
		xs := make([]float64, n)
		for i := range xs {
			switch v := r.IntN(20); {
			case v == 0:
				xs[i] = math.NaN()
			case v == 1:
				xs[i] = math.Inf(1 - 2*r.IntN(2))
			case v < 8:
				xs[i] = float64(r.IntN(5)) // ties
			default:
				xs[i] = r.NormFloat64() * 100
			}
		}
		for _, p := range []float64{0, 0.1, 1, 25, 50, 90, 99, 99.9, 100, r.Float64() * 100} {
			sorted := append([]float64(nil), xs...)
			sort.Float64s(sorted)
			want := PercentileSorted(sorted, p)
			got := percentileInPlace(append([]float64(nil), xs...), p)
			if !same(got, want) {
				t.Fatalf("n=%d p=%v: got %v, want %v (input %v)", n, p, got, want, xs)
			}
		}
	}
}

// The bootstrap estimators must be unchanged by the switch from sorting to
// selection: same seed, same CI, bit for bit.
func TestBootstrapMatchesSortedReference(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 8))
	var a, b, d []float64
	for i := 0; i < 200; i++ {
		a = append(a, 50+5*r.ExpFloat64())
		b = append(b, 52+5*r.ExpFloat64())
		d = append(d, float64(r.IntN(9))-3)
	}
	for _, p := range []float64{50, 90, 99} {
		if got, want := DiffOfPercentiles(a, b, p, 0.95, 11), refDiffOfPercentiles(a, b, p, 0.95, 11); got != want {
			t.Errorf("DiffOfPercentiles p%v = %+v, want %+v", p, got, want)
		}
	}
	if got, want := MedianOfPaired(d, 0.95, 3), refMedianOfPaired(d, 0.95, 3); got != want {
		t.Errorf("MedianOfPaired = %+v, want %+v", got, want)
	}
}

// refDiffOfPercentiles and refMedianOfPaired are the sort-based originals.
func refDiffOfPercentiles(a, b []float64, p, level float64, seed uint64) CI {
	est := Percentile(b, p) - Percentile(a, p)
	r := rand.New(rand.NewPCG(seed, 0x9e3779b97f4a7c15))
	a, b = subsample(a, r), subsample(b, r)
	diffs := make([]float64, resamples)
	ra := make([]float64, len(a))
	rb := make([]float64, len(b))
	for i := range diffs {
		for j := range ra {
			ra[j] = a[r.IntN(len(a))]
		}
		for j := range rb {
			rb[j] = b[r.IntN(len(b))]
		}
		sort.Float64s(ra)
		sort.Float64s(rb)
		diffs[i] = PercentileSorted(rb, p) - PercentileSorted(ra, p)
	}
	return interval(est, diffs, level)
}

func refMedianOfPaired(diffs []float64, level float64, seed uint64) CI {
	est := Percentile(diffs, 50)
	r := rand.New(rand.NewPCG(seed, 0x9e3779b97f4a7c15))
	d := subsample(diffs, r)
	boot := make([]float64, resamples)
	rs := make([]float64, len(d))
	for i := range boot {
		for j := range rs {
			rs[j] = d[r.IntN(len(d))]
		}
		sort.Float64s(rs)
		boot[i] = PercentileSorted(rs, 50)
	}
	return interval(est, boot, level)
}
