package calibration

import "math"

// Interval is a closed confidence interval on a rate in [0,1].
type Interval struct {
	Estimate float64 `json:"estimate"`
	Low      float64 `json:"low"`
	High     float64 `json:"high"`
	Level    float64 `json:"level"`
	Method   string  `json:"method"`
	N        int     `json:"n"`
	K        int     `json:"k"`
}

// WilsonInterval returns the Wilson score interval for k successes in n trials.
// z defaults to ~1.96 (95%) when level is 0.95; other common levels supported.
func WilsonInterval(k, n int, level float64) Interval {
	if level <= 0 || level >= 1 {
		level = 0.95
	}
	z := zForLevel(level)
	out := Interval{Level: level, Method: "wilson", N: n, K: k}
	if n <= 0 {
		out.Estimate = 0
		out.Low = 0
		out.High = 1
		return out
	}
	if k < 0 {
		k = 0
	}
	if k > n {
		k = n
	}
	phat := float64(k) / float64(n)
	out.Estimate = phat
	z2 := z * z
	den := 1 + z2/float64(n)
	center := phat + z2/(2*float64(n))
	margin := z * math.Sqrt((phat*(1-phat)+z2/(4*float64(n)))/float64(n))
	out.Low = clamp01((center - margin) / den)
	out.High = clamp01((center + margin) / den)
	return out
}

func zForLevel(level float64) float64 {
	switch {
	case math.Abs(level-0.90) < 1e-9:
		return 1.6448536269514722
	case math.Abs(level-0.99) < 1e-9:
		return 2.5758293035489004
	default:
		return 1.959963984540054 // 0.95
	}
}
