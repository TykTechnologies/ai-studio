// Package stats holds the few estimators the report relies on: exact
// percentiles, and bootstrap confidence intervals for a difference of medians
// (independent samples) and for a median of paired differences.
package stats

import (
	"math"
	"math/rand/v2"
	"sort"
)

// Percentile returns the p-th percentile (0-100) of xs by linear
// interpolation between closest ranks. xs is not modified. NaN when empty.
func Percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	return PercentileSorted(s, p)
}

// PercentileSorted is Percentile for an already sorted slice.
func PercentileSorted(s []float64, p float64) float64 {
	if len(s) == 0 {
		return math.NaN()
	}
	if p <= 0 {
		return s[0]
	}
	if p >= 100 {
		return s[len(s)-1]
	}
	rank := p / 100 * float64(len(s)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	frac := rank - float64(lo)
	return s[lo] + (s[hi]-s[lo])*frac
}

// Summary is the percentile set every table reports.
type Summary struct {
	N     int     `json:"n"`
	Mean  float64 `json:"mean"`
	P50   float64 `json:"p50"`
	P90   float64 `json:"p90"`
	P99   float64 `json:"p99"`
	P999  float64 `json:"p99_9"`
	Max   float64 `json:"max"`
	Min   float64 `json:"min"`
	Stdev float64 `json:"stdev"`
}

// Summarize sorts a copy of xs once and reads every percentile from it.
func Summarize(xs []float64) Summary {
	if len(xs) == 0 {
		nan := math.NaN()
		return Summary{Mean: nan, P50: nan, P90: nan, P99: nan, P999: nan, Max: nan, Min: nan, Stdev: nan}
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	var sum float64
	for _, x := range s {
		sum += x
	}
	mean := sum / float64(len(s))
	var ss float64
	for _, x := range s {
		ss += (x - mean) * (x - mean)
	}
	return Summary{
		N: len(s), Mean: mean,
		P50: PercentileSorted(s, 50), P90: PercentileSorted(s, 90), P99: PercentileSorted(s, 99),
		P999: PercentileSorted(s, 99.9), Max: s[len(s)-1], Min: s[0],
		Stdev: math.Sqrt(ss / float64(len(s))),
	}
}

// CI is a point estimate with a two-sided confidence interval.
type CI struct {
	Estimate float64 `json:"estimate"`
	Lo       float64 `json:"lo"`
	Hi       float64 `json:"hi"`
	// Level is the confidence level, e.g. 0.95.
	Level float64 `json:"level"`
}

// Contains reports whether v lies within the interval.
func (c CI) Contains(v float64) bool { return v >= c.Lo && v <= c.Hi }

// bootstrap settings: 2000 resamples of at most 5000 points per side keeps a
// report over millions of results to seconds, with CI endpoints stable to a
// few hundredths of a millisecond for the sample sizes these runs produce.
const (
	resamples = 2000
	maxPoints = 5000
)

// DiffOfPercentiles estimates percentile(b) - percentile(a) for independent
// samples, with a percentile-bootstrap CI. seed makes reports reproducible.
func DiffOfPercentiles(a, b []float64, p, level float64, seed uint64) CI {
	if len(a) == 0 || len(b) == 0 {
		nan := math.NaN()
		return CI{Estimate: nan, Lo: nan, Hi: nan, Level: level}
	}
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
		diffs[i] = percentileInPlace(rb, p) - percentileInPlace(ra, p)
	}
	return interval(est, diffs, level)
}

// MedianOfPaired estimates the median of per-pair differences with a
// bootstrap CI over pairs.
func MedianOfPaired(diffs []float64, level float64, seed uint64) CI {
	if len(diffs) == 0 {
		nan := math.NaN()
		return CI{Estimate: nan, Lo: nan, Hi: nan, Level: level}
	}
	est := Percentile(diffs, 50)
	r := rand.New(rand.NewPCG(seed, 0x9e3779b97f4a7c15))
	d := subsample(diffs, r)
	boot := make([]float64, resamples)
	rs := make([]float64, len(d))
	for i := range boot {
		for j := range rs {
			rs[j] = d[r.IntN(len(d))]
		}
		boot[i] = percentileInPlace(rs, 50)
	}
	return interval(est, boot, level)
}

// percentileInPlace returns PercentileSorted of s sorted, bit for bit, but
// selects the two ranks it reads instead of sorting: O(n) rather than
// O(n log n), which is most of a bootstrap's time. s is reordered.
func percentileInPlace(s []float64, p float64) float64 {
	n := len(s)
	if n == 0 {
		return math.NaN()
	}
	if p <= 0 || p >= 100 {
		k := 0
		for i := 1; i < n; i++ {
			if (p <= 0 && floatLess(s[i], s[k])) || (p >= 100 && floatLess(s[k], s[i])) {
				k = i
			}
		}
		return s[k]
	}
	rank := p / 100 * float64(n-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	frac := rank - float64(lo)
	selectKth(s, lo)
	sHi := s[lo]
	if hi != lo {
		// After selectKth everything right of lo ranks at or above it, so
		// the next rank is their minimum.
		k := lo + 1
		for i := lo + 2; i < n; i++ {
			if floatLess(s[i], s[k]) {
				k = i
			}
		}
		sHi = s[k]
	}
	return s[lo] + (sHi-s[lo])*frac
}

// floatLess is the order sort.Float64s uses: NaNs before everything else.
func floatLess(a, b float64) bool {
	return a < b || (math.IsNaN(a) && !math.IsNaN(b))
}

// selectKth reorders s so that s[k] holds the element a sort would put
// there, nothing left of k ranks above it and nothing right of k below it
// (Hoare's quickselect with a median-of-three pivot).
func selectKth(s []float64, k int) {
	lo, hi := 0, len(s)-1
	for hi-lo > 16 {
		mid := lo + (hi-lo)/2
		if floatLess(s[mid], s[lo]) {
			s[mid], s[lo] = s[lo], s[mid]
		}
		if floatLess(s[hi], s[lo]) {
			s[hi], s[lo] = s[lo], s[hi]
		}
		if floatLess(s[hi], s[mid]) {
			s[hi], s[mid] = s[mid], s[hi]
		}
		pivot := s[mid]
		i, j := lo, hi
		for i <= j {
			for floatLess(s[i], pivot) {
				i++
			}
			for floatLess(pivot, s[j]) {
				j--
			}
			if i <= j {
				s[i], s[j] = s[j], s[i]
				i++
				j--
			}
		}
		// Now s[lo..j] <= pivot <= s[i..hi], and s[j+1..i-1] == pivot.
		switch {
		case k <= j:
			hi = j
		case k >= i:
			lo = i
		default:
			return
		}
	}
	// Insertion sort the short remainder.
	for i := lo + 1; i <= hi; i++ {
		for j := i; j > lo && floatLess(s[j], s[j-1]); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func interval(est float64, boot []float64, level float64) CI {
	sort.Float64s(boot)
	alpha := (1 - level) / 2
	return CI{
		Estimate: est,
		Lo:       PercentileSorted(boot, 100*alpha),
		Hi:       PercentileSorted(boot, 100*(1-alpha)),
		Level:    level,
	}
}

// subsample draws at most maxPoints without replacement.
func subsample(xs []float64, r *rand.Rand) []float64 {
	if len(xs) <= maxPoints {
		return xs
	}
	out := append([]float64(nil), xs...)
	r.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out[:maxPoints]
}

// Slope is the least-squares slope of y over x (per unit of x). Used to spot
// leaks: RSS or goroutines growing steadily through a soak.
func Slope(x, y []float64) float64 {
	n := float64(len(x))
	if len(x) < 2 || len(x) != len(y) {
		return math.NaN()
	}
	var sx, sy, sxx, sxy float64
	for i := range x {
		sx += x[i]
		sy += y[i]
		sxx += x[i] * x[i]
		sxy += x[i] * y[i]
	}
	den := n*sxx - sx*sx
	if den == 0 {
		return math.NaN()
	}
	return (n*sxy - sx*sy) / den
}
