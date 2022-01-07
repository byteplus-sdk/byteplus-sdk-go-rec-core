package metrics

// Copyright 2012 Richard Crowley. All rights reserved.
// <https://github.com/rcrowley/go-metrics>

import (
	"math"
	"math/rand"
	"sort"
	"sync"
)

// sample Samples maintain a statistically-significant selection of values from
// a stream.
type sample interface {
	clear()
	count() int64
	max() int64
	mean() float64
	min() int64
	percentile(float64) float64
	percentiles([]float64) []float64
	size() int
	snapshot() sample
	stdDev() float64
	sum() int64
	update(int64)
	values() []int64
	variance() float64
}

// sampleMax returns the maximum value of the slice of int64.
func sampleMax(values []int64) int64 {
	if 0 == len(values) {
		return 0
	}
	var max int64 = math.MinInt64
	for _, v := range values {
		if max < v {
			max = v
		}
	}
	return max
}

// sampleMean returns the mean value of the slice of int64.
func sampleMean(values []int64) float64 {
	if 0 == len(values) {
		return 0.0
	}
	return float64(sampleSum(values)) / float64(len(values))
}

// sampleMin returns the minimum value of the slice of int64.
func sampleMin(values []int64) int64 {
	if 0 == len(values) {
		return 0
	}
	var min int64 = math.MaxInt64
	for _, v := range values {
		if min > v {
			min = v
		}
	}
	return min
}

// samplePercentile samplePercentiles returns an arbitrary percentile of the slice of int64.
func samplePercentile(values int64Slice, p float64) float64 {
	return samplePercentiles(values, []float64{p})[0]
}

// samplePercentiles returns a slice of arbitrary percentiles of the slice of
// int64.
func samplePercentiles(values int64Slice, ps []float64) []float64 {
	scores := make([]float64, len(ps))
	size := len(values)
	if size > 0 {
		sort.Sort(values)
		for i, p := range ps {
			pos := p * float64(size+1)
			if pos < 1.0 {
				scores[i] = float64(values[0])
			} else if pos >= float64(size) {
				scores[i] = float64(values[size-1])
			} else {
				lower := float64(values[int(pos)-1])
				upper := float64(values[int(pos)])
				scores[i] = lower + (pos-math.Floor(pos))*(upper-lower)
			}
		}
	}
	return scores
}

// sampleSnapshot is a read-only copy of another Sample.
type sampleSnapshot struct {
	mCount  int64
	mValues []int64
}

func newSampleSnapshot(count int64, values []int64) *sampleSnapshot {
	return &sampleSnapshot{
		mCount:  count,
		mValues: values,
	}
}

// clear panics.
func (*sampleSnapshot) clear() {
	panic("Clear called on a SampleSnapshot")
}

// count returns the count of inputs at the time the snapshot was taken.
func (s *sampleSnapshot) count() int64 { return s.mCount }

// max returns the maximal value at the time the snapshot was taken.
func (s *sampleSnapshot) max() int64 { return sampleMax(s.mValues) }

// mean returns the mean value at the time the snapshot was taken.
func (s *sampleSnapshot) mean() float64 { return sampleMean(s.mValues) }

// min returns the minimal value at the time the snapshot was taken.
func (s *sampleSnapshot) min() int64 { return sampleMin(s.mValues) }

// percentile returns an arbitrary percentile of values at the time the
// snapshot was taken.
func (s *sampleSnapshot) percentile(p float64) float64 {
	return samplePercentile(s.mValues, p)
}

// percentiles returns a slice of arbitrary percentiles of values at the time
// the snapshot was taken.
func (s *sampleSnapshot) percentiles(ps []float64) []float64 {
	return samplePercentiles(s.mValues, ps)
}

// size returns the size of the sample at the time the snapshot was taken.
func (s *sampleSnapshot) size() int { return len(s.mValues) }

// snapshot returns the snapshot.
func (s *sampleSnapshot) snapshot() sample { return s }

// stdDev returns the standard deviation of values at the time the snapshot was
// taken.
func (s *sampleSnapshot) stdDev() float64 { return sampleStdDev(s.mValues) }

// sum returns the sum of values at the time the snapshot was taken.
func (s *sampleSnapshot) sum() int64 { return sampleSum(s.mValues) }

// update panics.
func (*sampleSnapshot) update(int64) {
	panic("Update called on a SampleSnapshot")
}

// Values returns a copy of the values in the sample.
func (s *sampleSnapshot) values() []int64 {
	values := make([]int64, len(s.mValues))
	copy(values, s.mValues)
	return values
}

// variance returns the variance of values at the time the snapshot was taken.
func (s *sampleSnapshot) variance() float64 { return sampleVariance(s.mValues) }

// sampleStdDev returns the standard deviation of the slice of int64.
func sampleStdDev(values []int64) float64 {
	return math.Sqrt(sampleVariance(values))
}

// sampleSum returns the sum of the slice of int64.
func sampleSum(values []int64) int64 {
	var sum int64
	for _, v := range values {
		sum += v
	}
	return sum
}

// sampleVariance returns the variance of the slice of int64.
func sampleVariance(values []int64) float64 {
	if 0 == len(values) {
		return 0.0
	}
	m := sampleMean(values)
	var sum float64
	for _, v := range values {
		d := float64(v) - m
		sum += d * d
	}
	return sum / float64(len(values))
}

// uniformSample A uniform sample using Vitter's Algorithm R.
//
// <http://www.cs.umd.edu/~samir/498/vitter.pdf>
type uniformSample struct {
	mCount        int64
	mutex         sync.Mutex
	reservoirSize int
	mValues       []int64
}

// newUniformSample constructs a new uniform sample with the given reservoir
// size.
func newUniformSample(reservoirSize int) sample {
	return &uniformSample{
		reservoirSize: reservoirSize,
		mValues:       make([]int64, 0, reservoirSize),
	}
}

// clear clears all samples.
func (s *uniformSample) clear() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.mCount = 0
	s.mValues = make([]int64, 0, s.reservoirSize)
}

// count returns the number of samples recorded, which may exceed the
// reservoir size.
func (s *uniformSample) count() int64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.mCount
}

// Max returns the maximum value in the sample, which may not be the maximum
// value ever to be part of the sample.
func (s *uniformSample) max() int64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return sampleMax(s.mValues)
}

// mean returns the mean of the values in the sample.
func (s *uniformSample) mean() float64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return sampleMean(s.mValues)
}

// min returns the minimum value in the sample, which may not be the minimum
// value ever to be part of the sample.
func (s *uniformSample) min() int64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return sampleMin(s.mValues)
}

// percentile returns an arbitrary percentile of values in the sample.
func (s *uniformSample) percentile(p float64) float64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return samplePercentile(s.mValues, p)
}

// percentiles returns a slice of arbitrary percentiles of values in the
// sample.
func (s *uniformSample) percentiles(ps []float64) []float64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return samplePercentiles(s.mValues, ps)
}

// size returns the size of the sample, which is at most the reservoir size.
func (s *uniformSample) size() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return len(s.mValues)
}

// snapshot returns a read-only copy of the sample.
func (s *uniformSample) snapshot() sample {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	values := make([]int64, len(s.mValues))
	copy(values, s.mValues)
	return &sampleSnapshot{
		mCount:  s.mCount,
		mValues: values,
	}
}

// stdDev returns the standard deviation of the values in the sample.
func (s *uniformSample) stdDev() float64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return sampleStdDev(s.mValues)
}

// sum returns the sum of the values in the sample.
func (s *uniformSample) sum() int64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return sampleSum(s.mValues)
}

// update samples a new value.
func (s *uniformSample) update(v int64) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.mCount++
	if len(s.mValues) < s.reservoirSize {
		s.mValues = append(s.mValues, v)
	} else {
		r := rand.Int63n(s.mCount)
		if r < int64(len(s.mValues)) {
			s.mValues[int(r)] = v
		}
	}
}

// values returns a copy of the values in the sample.
func (s *uniformSample) values() []int64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	values := make([]int64, len(s.mValues))
	copy(values, s.mValues)
	return values
}

// variance returns the variance of the values in the sample.
func (s *uniformSample) variance() float64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return sampleVariance(s.mValues)
}

// expDecaySample represents an individual sample in a heap.
type expDecaySample struct {
	k float64
	v int64
}

func newExpDecaySampleHeap(reservoirSize int) *expDecaySampleHeap {
	return &expDecaySampleHeap{make([]expDecaySample, 0, reservoirSize)}
}

// expDecaySampleHeap is a min-heap of expDecaySamples.
// The internal implementation is copied from the standard library's container/heap
type expDecaySampleHeap struct {
	s []expDecaySample
}

func (h *expDecaySampleHeap) clear() {
	h.s = h.s[:0]
}

func (h *expDecaySampleHeap) push(s expDecaySample) {
	n := len(h.s)
	h.s = h.s[0 : n+1]
	h.s[n] = s
	h.up(n)
}

func (h *expDecaySampleHeap) pop() expDecaySample {
	n := len(h.s) - 1
	h.s[0], h.s[n] = h.s[n], h.s[0]
	h.down(0, n)

	n = len(h.s)
	s := h.s[n-1]
	h.s = h.s[0 : n-1]
	return s
}

func (h *expDecaySampleHeap) size() int {
	return len(h.s)
}

func (h *expDecaySampleHeap) values() []expDecaySample {
	return h.s
}

func (h *expDecaySampleHeap) up(j int) {
	for {
		i := (j - 1) / 2 // parent
		if i == j || !(h.s[j].k < h.s[i].k) {
			break
		}
		h.s[i], h.s[j] = h.s[j], h.s[i]
		j = i
	}
}

func (h *expDecaySampleHeap) down(i, n int) {
	for {
		j1 := 2*i + 1
		if j1 >= n || j1 < 0 { // j1 < 0 after int overflow
			break
		}
		j := j1 // left child
		if j2 := j1 + 1; j2 < n && !(h.s[j1].k < h.s[j2].k) {
			j = j2 // = 2*i + 2  // right child
		}
		if !(h.s[j].k < h.s[i].k) {
			break
		}
		h.s[i], h.s[j] = h.s[j], h.s[i]
		i = j
	}
}

type int64Slice []int64

func (p int64Slice) Len() int           { return len(p) }
func (p int64Slice) Less(i, j int) bool { return p[i] < p[j] }
func (p int64Slice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
