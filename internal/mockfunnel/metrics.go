package mockfunnel

import (
	"sort"
	"sync"
	"time"
)

// latency ring to compute percentiles without unbounded memory.
type latencyRing struct {
	mu    sync.Mutex
	vals  []int // milliseconds
	pos   int
	count int
}

func newLatencyRing(size int) *latencyRing {
	return &latencyRing{vals: make([]int, size)}
}

func (r *latencyRing) add(v int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.vals[r.pos] = v
	r.pos = (r.pos + 1) % len(r.vals)
	if r.count < len(r.vals) {
		r.count++
	}
}

func (r *latencyRing) snapshot() []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]int, r.count)
	// Copy respecting ring wrap-around
	if r.count < len(r.vals) {
		copy(out, r.vals[:r.count])
		return out
	}
	copy(out, r.vals[r.pos:])
	copy(out[len(r.vals)-r.pos:], r.vals[:r.pos])
	return out
}

// percentile returns the p in [0,100], or -1 if empty.
func percentile(vals []int, p float64) int {
	if len(vals) == 0 {
		return -1
	}
	s := make([]int, len(vals))
	copy(s, vals)
	sort.Ints(s)
	if p <= 0 {
		return s[0]
	}
	if p >= 100 {
		return s[len(s)-1]
	}
	idx := int(float64(len(s)-1) * p / 100.0)
	return s[idx]
}

type secondBucket struct {
	Requests int
	Success  int
	Errors   int
	Timeouts int
	LatencyTotalMs int
}

type slidingWindow struct {
	mu      sync.Mutex
	window  map[int64]*secondBucket // key: unix sec
	span    int                      // seconds to keep
}

func newSlidingWindow(span int) *slidingWindow {
	return &slidingWindow{window: make(map[int64]*secondBucket), span: span}
}

func (w *slidingWindow) add(now time.Time, latencyMs int, ok, isTimeout bool) {
	sec := now.Unix()
	w.mu.Lock()
	b := w.window[sec]
	if b == nil {
		b = &secondBucket{}
		w.window[sec] = b
	}
	b.Requests++
	if ok {
		b.Success++
	} else {
		if isTimeout {
			b.Timeouts++
		} else {
			b.Errors++
		}
	}
	b.LatencyTotalMs += latencyMs
	// GC old buckets
	cut := sec - int64(w.span)
	for k := range w.window {
		if k < cut {
			delete(w.window, k)
		}
	}
	w.mu.Unlock()
}

func (w *slidingWindow) snapshot() (secs []int64, rps []int, avgLatency []int, succ []int, errs []int, timeouts []int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.window) == 0 {
		return
	}
	// collect last w.span seconds in order
	end := time.Now().Unix()
	start := end - int64(w.span) + 1
	for s := start; s <= end; s++ {
		b := w.window[s]
		secs = append(secs, s)
		if b == nil {
			rps = append(rps, 0)
			avgLatency = append(avgLatency, 0)
			succ = append(succ, 0)
			errs = append(errs, 0)
			timeouts = append(timeouts, 0)
			continue
		}
		rps = append(rps, b.Requests)
		if b.Requests > 0 {
			avgLatency = append(avgLatency, b.LatencyTotalMs/b.Requests)
		} else {
			avgLatency = append(avgLatency, 0)
		}
		succ = append(succ, b.Success)
		errs = append(errs, b.Errors)
		timeouts = append(timeouts, b.Timeouts)
	}
	return
}

type LineMetrics struct {
	Requests int64 `json:"requests"`
	Success  int64 `json:"success"`
	Errors   int64 `json:"errors"`
	Timeouts int64 `json:"timeouts"`
	P50 int `json:"p50_ms"`
	P95 int `json:"p95_ms"`
	P99 int `json:"p99_ms"`

	Ring *latencyRing     `json:"-"`
	Win  *slidingWindow   `json:"-"`

	mu sync.Mutex
}

func newLineMetrics() *LineMetrics {
	return &LineMetrics{
		Ring: newLatencyRing(1024),
		Win:  newSlidingWindow(60),
	}
}

func (m *LineMetrics) addSample(now time.Time, latencyMs int, ok, isTimeout bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Requests++
	if ok {
		m.Success++
	} else {
		if isTimeout {
			m.Timeouts++
		} else {
			m.Errors++
		}
	}
	m.Ring.add(latencyMs)
	m.Win.add(now, latencyMs, ok, isTimeout)

	vals := m.Ring.snapshot()
	m.P50 = percentile(vals, 50)
	m.P95 = percentile(vals, 95)
	m.P99 = percentile(vals, 99)
}

func (m *LineMetrics) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	*m = *newLineMetrics()
}
