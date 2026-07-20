package services

import "math"

// SignificanceAlpha is the two-sided p-value threshold below which the
// A/B-test rate-leader is declared the winner over the aggregated rest.
// ponytail: single peeking knob — repeated per-order checks trade strict
// Type-I control for responsiveness (acceptable: a wrong call only soft-
// deactivates a landing page, fully reversible). Drop to 0.01 if a merchant
// reports flapping decisions.
const SignificanceAlpha = 0.05

// normalCDF is the standard-normal CDF via the complementary error function.
func normalCDF(x float64) float64 {
	return 0.5 * math.Erfc(-x/math.Sqrt2)
}

// TwoProportionZTest is a pooled two-proportion z-test comparing two
// conversion rates (conv/n each). Returns the z statistic and the two-sided
// p-value. n1/n2 of 0 means no evidence either way, so p is 1 (never a false
// winner on empty data).
// ponytail: no continuity correction — the target_conversions floor upstream
// keeps n large enough that it's negligible.
func TwoProportionZTest(conv1, n1, conv2, n2 int) (z, pValue float64) {
	if n1 == 0 || n2 == 0 {
		return 0, 1
	}
	p1 := float64(conv1) / float64(n1)
	p2 := float64(conv2) / float64(n2)
	pPool := float64(conv1+conv2) / float64(n1+n2)
	se := math.Sqrt(pPool * (1 - pPool) * (1.0/float64(n1) + 1.0/float64(n2)))
	if se == 0 {
		return 0, 1
	}
	z = (p1 - p2) / se
	pValue = 2 * (1 - normalCDF(math.Abs(z)))
	return z, pValue
}
