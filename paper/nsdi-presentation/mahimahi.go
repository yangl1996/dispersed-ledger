package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
)

// every parameter is ms or KB/s
// X[n+1] = alpha * X[n] + N(0, stddev^2)
func GaussMarkov(duration, sampleIntv int, mean, alpha, stddev float64) {
	bws := []float64{0.0, 0.0, 0.0, 0.0}
	currentTime := 0.0
	normFactor := math.Sqrt(1 - alpha*alpha)
	fmt.Println("# time node1..4 bottleneck max")

	for currentTime < float64(duration) {
		for i := range bws {
			bws[i] = alpha * bws[i] + rand.NormFloat64()*stddev*normFactor
		}
		copied := make([]float64, 4)
		copy(copied, bws)
		sort.Float64s(copied)
		fmt.Printf("%.2f %.2f %.2f %.2f %.2f %.2f %.2f\n", currentTime, bws[0]+mean, bws[1]+mean, bws[2]+mean, bws[3]+mean, copied[1]+mean, copied[3]+mean)
		currentTime += float64(sampleIntv)
	}
}

func main() {
	duration := 600000
	interval := 1000
	mean := 15000.0
	variance := 4000.0
	alpha := 0.98

	GaussMarkov(duration, interval, mean, alpha, variance)
}
