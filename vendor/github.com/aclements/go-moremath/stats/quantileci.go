// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stats

import (
	"fmt"
	"math"
)

// QuantileCIResult is the confidence interval for a quantile.
type QuantileCIResult struct {
	// Quantile is the quantile of this confidence interval. This
	// is simply a copy of the argument to QuantileCI.
	Quantile float64

	// N is the sample size.
	N int

	// Confidence is the actual confidence level of this interval.
	// This will be >= the requested confidence.
	Confidence float64

	// LoOrder and HiOrder are the order statistics that bound the
	// confidence interval. By convention, these are 1-based, so
	// given an ordered slice of samples Xs, the CI is
	// Xs[LoOrder-1] to Xs[HiOrder-1].
	//
	// These may be outside the range of the sample, which
	// indicates that corresponding bound is negative or positive
	// infinity. This can happen, for example, if the sample is
	// too small for a high confidence level, or the quantile is
	// close to 0 or 1.
	LoOrder, HiOrder int

	// Ambiguous indicates that the given confidence interval is
	// ambiguous. In this case, the interval LoOrder+1 to
	// HiOrder+1 has equivalent confidence.
	Ambiguous bool
}

// SampleCI returns the quantile and its confidence interval for a
// sample given the parameters in ci. It may return negative or
// positive infinity if the interval lies outside the sample.
func (ci QuantileCIResult) SampleCI(s Sample) (q, lo, hi float64) {
	if s.Weights != nil {
		panic("Cannot compute quantile CI on a weighted sample")
	}
	if len(s.Xs) != ci.N {
		panic("Sample size differs from computed quantile CI")
	}

	if !s.Sorted {
		s = *s.Copy().Sort()
	}

	q = s.Quantile(ci.Quantile)
	if ci.LoOrder < 1 {
		// The sample is too small or the confidence is too high.
		lo = math.Inf(-1)
	} else {
		lo = s.Xs[ci.LoOrder-1]
	}
	if ci.HiOrder-1 >= len(s.Xs) {
		hi = math.Inf(1)
	} else {
		hi = s.Xs[ci.HiOrder-1]
	}
	return
}

// quantileCIApproxThreshold is the threshold above which a normal
// approximation is used. This is a variable for testing.
//
// Performance-wise, these cross over at about n=5, but the
// approximation isn't very good at low n.
var quantileCIApproxThreshold = 30

// QuantileCI returns the bounds of the confidence interval of the
// q'th quantile in a sample of size n.
func QuantileCI(n int, q, confidence float64) QuantileCIResult {
	const debug = false

	var res QuantileCIResult
	res.N = n
	res.Quantile = q

	if confidence >= 1 {
		res.Confidence = 1
		res.LoOrder = 0
		res.HiOrder = n + 1
		return res
	}

	if debug {
		fmt.Printf("QuantileCI(%v, %v, %v)\n", n, q, confidence)
	}

	// There's a dearth of good information online about how to
	// compute this, especially in corner cases. Some useful
	// online resources:
	//
	// https://online.stat.psu.edu/stat415/book/export/html/835 -
	// The concept of intervals, some worked examples.
	//
	// http://www.milefoot.com/math/stat/ci-medians.htm - Good
	// walk through of summing up binomial probabilities,
	// continuity correction for the normal approximation.

	// The sampling distribution for order statistics is the
	// binomial distribution. In this distribution, k is how many
	// samples come before the population median; or,
	// alternatively, an index into the intervals between samples
	// (where 0 is the interval from -∞ to the first sample).
	// Hence, PMF(k) gives the probability that the population
	// median falls in interval k, or between s.Xs[k-1] and
	// s.Xs[k].
	samp := BinomialDist{N: n, P: q}

	// l and r are the left and right order statistics of the
	// confidence interval.
	var l, r int
	if samp.N <= quantileCIApproxThreshold {
		if debug {
			for i := 0; i <= samp.N; i++ {
				fmt.Printf("  %d | %v\n", i, samp.PMF(float64(i)))
			}
		}

		// Start with the mode and accumulate probabilities in
		// decreasing order until we pass the confidence
		// level. This uses the fact that the probabilities
		// decrease monotonically as you move out from the
		// mode.
		//
		// The binomial distribution can be have equal modes.
		// Since we want to left-bias our result, we start
		// with the lower of the two.
		x := int(math.Ceil(float64(samp.N+1)*samp.P) - 1)
		if samp.P == 0 { // Special case of the mode
			x = 0
		}
		accum := samp.PMF(float64(x))
		if debug {
			fmt.Printf("  start %d => %v\n", x, accum)
		}

		// Compute the neighboring probabilities so we can
		// incrementally add and update them. [l, r) is the
		// interval we've summed.
		l, r = x, x+1
		lp, rp := samp.PMF(float64(l-1)), samp.PMF(float64(r))
		// If the binomial distribution has two modes, then
		// our initial selection is ambiguous.
		res.Ambiguous = rp == accum

		// Accumulate probabilities to reach the desired
		// confidence level. We defend against accumulation
		// errors by stopping if there's no more to
		// accumulate.
		//
		// For the particular case of q=0.5, the distribution
		// is symmetric and we could just use InvCDF like we
		// do in the normal approximation. But that doesn't
		// generalize to other quantiles, and InvCDF isn't
		// particularly efficient on the binomial distribution
		// anyway.
		for accum < confidence && (lp > 0 || rp > 0) {
			res.Ambiguous = lp == rp
			if lp >= rp { // Left-bias
				accum += lp
				if debug {
					fmt.Printf("  +left  %d => %v\n", l-1, accum)
				}
				l--
				lp = samp.PMF(float64(l - 1))
			} else {
				accum += rp
				if debug {
					fmt.Printf("  +right %d => %v\n", r, accum)
				}
				r++
				rp = samp.PMF(float64(r))
			}
		}
		res.Confidence = accum

		if debug {
			fmt.Printf("  final [%d,%d) => %v (ambiguous %v)\n", l, r, accum, res.Ambiguous)
		}
	} else {
		// Use the normal approximation.
		norm := samp.NormalApprox()
		alpha := (1 - confidence) / 2

		// Find the center "confidence" weight of the
		// distribution.
		l1 := norm.InvCDF(alpha)
		r1 := 2*norm.Mu - l1 // Symmetric around mean.

		// Find the band of the discrete binomial distribution
		// containing [l1, r1]. Because of the continuity
		// correction, point k in the binomial distribution
		// corresponds to band [k-0.5, k+0.5] in the normal
		// distribution. Hence, we round out to ℕ + 0.5
		// boundaries and then recover k.
		//
		// For example, let's say mu=2 and confidence is
		// really low. If [l1, r1] is [1.9, 2.1], that rounds
		// out to [1.5, 2.5], which is the band [2, 3) in the
		// binomial distribution. But if [l1, r1] is [1.4,
		// 2.6], that rounds out to [0.5, 3.5], which is the
		// band [1, 4) in the binomial distribution.
		floorInt := func(x float64) int {
			// int(x) truncates toward 0, so floor first.
			return int(math.Floor(x))
		}
		l = floorInt(math.Floor(l1-0.5)+0.5) + 1
		r = floorInt(math.Ceil(r1-0.5)+0.5) + 1

		if debug {
			fmt.Printf("  [%v,%v] rounds to [%v,%v]\n", l1, r1, l, r)
		}

		// The actual confidence on the binomial
		// distribution is
		//
		//   Pr[l <= X < r] = Pr[X <= r - 1] - Pr[X <= l - 1]
		//
		// To translate this into the normal
		// approximation, we add 0.5 to each bound for
		// the continuity correction.
		cdf := func(l, r int) float64 {
			return norm.CDF(float64(r)-0.5) - norm.CDF(float64(l)-0.5)
		}
		res.Confidence = cdf(l, r)
		// The computed interval is always symmetric.
		// Try left-biasing it and see if we can do
		// better while still satisfying the
		// confidence level.
		rBiased := r - 1
		if debug {
			fmt.Printf("  unbiased %v, biased %v\n", res.Confidence, cdf(l, rBiased))
		}
		if aBiased := cdf(l, rBiased); aBiased >= confidence && aBiased < res.Confidence {
			if debug {
				fmt.Printf("  taking biased\n")
			}
			res.Confidence, res.Ambiguous = aBiased, true
			r = rBiased
		}
		if l <= 0 && r >= n+1 {
			// The CI covers everything, but
			// because the normal distribution has
			// infinite support, the confidence
			// computed by CDF won't be quite 1.
			// Certainly the median falls between
			// -inf and +inf. This can happen even
			// in the biasing case, so we check
			// this in any case.
			if debug {
				fmt.Printf("  adjusting for full range\n")
			}
			res.Confidence = 1
			res.Ambiguous = false
		}
	}

	if l < 0 {
		l = 0
	}
	if r > n+1 {
		r = n + 1
	}
	res.LoOrder, res.HiOrder = l, r
	return res
}
