package genaimetricsconnector

import (
	"sort"

	"go.opentelemetry.io/collector/pdata/pmetric"
)

// histogram is a minimal cumulative explicit-bucket histogram accumulator.
type histogram struct {
	bounds  []float64
	buckets []uint64 // len(bounds)+1; last bucket is (bounds[last], +Inf)
	count   uint64
	sum     float64
}

func newHistogram(bounds []float64) *histogram {
	return &histogram{bounds: bounds, buckets: make([]uint64, len(bounds)+1)}
}

// observe records a single measurement.
func (h *histogram) observe(v float64) {
	idx := sort.Search(len(h.bounds), func(i int) bool { return v <= h.bounds[i] })
	h.buckets[idx]++
	h.count++
	h.sum += v
}

// writeTo copies the current cumulative state into an OTLP histogram data point.
func (h *histogram) writeTo(dp pmetric.HistogramDataPoint) {
	dp.ExplicitBounds().FromRaw(h.bounds)
	dp.BucketCounts().FromRaw(h.buckets)
	dp.SetCount(h.count)
	dp.SetSum(h.sum)
}
