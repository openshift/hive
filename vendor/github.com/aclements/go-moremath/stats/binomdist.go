// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stats

import (
	"math"

	"github.com/aclements/go-moremath/mathx"
)

// BinomialDist is a binomial distribution.
type BinomialDist struct {
	// N is the number of independent Bernoulli trials. N >= 0.
	//
	// If N=1, this is equivalent to the Bernoulli distribution.
	N int

	// P is the probability of success in each trial. 0 <= P <= 1.
	P float64
}

// PMF is the probability of getting exactly int(k) successes in d.N
// independent Bernoulli trials with probability d.P.
func (d BinomialDist) PMF(k float64) float64 {
	ki := int(math.Floor(k))
	if ki < 0 || ki > d.N {
		return 0
	}
	return mathx.Choose(d.N, ki) * math.Pow(d.P, float64(ki)) * math.Pow(1-d.P, float64(d.N-ki))
}

// CDF is the probability of getting k or fewer successes in d.N
// independent Bernoulli trials with probability d.P.
func (d BinomialDist) CDF(k float64) float64 {
	k = math.Floor(k)
	ki := int(k)
	if ki < 0 {
		return 0
	} else if ki >= d.N {
		return 1
	}

	return mathx.BetaInc(1-d.P, float64(d.N-ki), k+1)
}

func (d BinomialDist) Bounds() (float64, float64) {
	return 0, float64(d.N)
}

func (d BinomialDist) Step() float64 {
	return 1
}

func (d BinomialDist) Mean() float64 {
	return float64(d.N) * d.P
}

func (d BinomialDist) Variance() float64 {
	return float64(d.N) * d.P * (1 - d.P)
}

// NormalApprox returns a normal distribution approximation of
// binomial distribution d.
//
// Because the binomial distribution is discrete and the normal
// distribution is continuous, the caller must apply a continuity
// correction when using this approximation. Specifically, if b is the
// binomial distribution and n is the normal approximation, operations
// map as follows:
//
//   b.PMF(k) => n.CDF(k+0.5) - n.CDF(k-0.5)
//   b.CDF(k) => n.CDF(k+0.5)
func (d BinomialDist) NormalApprox() NormalDist {
	return NormalDist{Mu: d.Mean(), Sigma: math.Sqrt(d.Variance())}
}
