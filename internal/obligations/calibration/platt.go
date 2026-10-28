package calibration

import (
	"math"
)

// LogisticPredict returns σ(w·x + b).
func LogisticPredict(weights []float64, bias float64, x []float64) float64 {
	if len(weights) != len(x) {
		return 0.5
	}
	z := bias
	for i := range weights {
		z += weights[i] * x[i]
	}
	return sigmoid(z)
}

func sigmoid(z float64) float64 {
	if z > 35 {
		return 1
	}
	if z < -35 {
		return 0
	}
	return 1 / (1 + math.Exp(-z))
}

// FitLogistic fits L2-regularized logistic regression with Newton–Raphson.
// Deterministic given identical inputs; no external deps.
func FitLogistic(xs [][]float64, ys []float64, l2 float64, maxIter int) (weights []float64, bias float64) {
	if len(xs) == 0 || len(xs) != len(ys) {
		return nil, 0
	}
	d := len(xs[0])
	weights = make([]float64, d)
	if maxIter <= 0 {
		maxIter = 40
	}
	if l2 <= 0 {
		l2 = 1e-2
	}
	// Working vector θ = [bias, w...]
	theta := make([]float64, d+1)
	grad := make([]float64, d+1)
	hess := make([][]float64, d+1)
	for i := range hess {
		hess[i] = make([]float64, d+1)
	}

	for iter := 0; iter < maxIter; iter++ {
		for i := range grad {
			grad[i] = 0
			for j := range hess[i] {
				hess[i][j] = 0
			}
		}
		// L2 on weights only (not bias)
		for j := 1; j <= d; j++ {
			grad[j] += l2 * theta[j]
			hess[j][j] += l2
		}
		for n, x := range xs {
			z := theta[0]
			for j := 0; j < d; j++ {
				z += theta[j+1] * x[j]
			}
			p := sigmoid(z)
			diff := p - ys[n]
			w := p * (1 - p)
			if w < 1e-8 {
				w = 1e-8
			}
			grad[0] += diff
			hess[0][0] += w
			for j := 0; j < d; j++ {
				grad[j+1] += diff * x[j]
				hess[0][j+1] += w * x[j]
				hess[j+1][0] += w * x[j]
				for k := 0; k < d; k++ {
					hess[j+1][k+1] += w * x[j] * x[k]
				}
			}
		}
		delta, ok := solveLinear(hess, grad)
		if !ok {
			break
		}
		stepNorm := 0.0
		for i := range theta {
			theta[i] -= delta[i]
			stepNorm += delta[i] * delta[i]
		}
		if stepNorm < 1e-12 {
			break
		}
	}
	bias = theta[0]
	copy(weights, theta[1:])
	return weights, bias
}

// solveLinear solves H δ = g via Gaussian elimination with partial pivoting.
func solveLinear(H [][]float64, g []float64) ([]float64, bool) {
	n := len(g)
	a := make([][]float64, n)
	for i := 0; i < n; i++ {
		a[i] = make([]float64, n+1)
		copy(a[i], H[i])
		a[i][n] = g[i]
	}
	for col := 0; col < n; col++ {
		pivot := col
		best := math.Abs(a[col][col])
		for r := col + 1; r < n; r++ {
			if v := math.Abs(a[r][col]); v > best {
				best = v
				pivot = r
			}
		}
		if best < 1e-14 {
			return nil, false
		}
		a[col], a[pivot] = a[pivot], a[col]
		div := a[col][col]
		for c := col; c <= n; c++ {
			a[col][c] /= div
		}
		for r := 0; r < n; r++ {
			if r == col {
				continue
			}
			factor := a[r][col]
			for c := col; c <= n; c++ {
				a[r][c] -= factor * a[col][c]
			}
		}
	}
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		out[i] = a[i][n]
	}
	return out, true
}
