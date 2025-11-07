package mockfunnel

import (
	"math/rand"
	"sync"
	"time"
)

type LineID string

const (
	LineOuterUnified LineID = "outer-unified"
	LineInnerUnified LineID = "inner-unified"
	LineOuterZf      LineID = "outer-zf"
	LineInnerZf      LineID = "inner-zf"
)

var AllLines = []LineID{LineOuterUnified, LineInnerUnified, LineOuterZf, LineInnerZf}

type TimeOfDay struct {
	Start string `json:"start"` // "HH:MM" 24h
	End   string `json:"end"`   // "HH:MM" 24h
}

type LineConfig struct {
	Name              string    `json:"name"`
	Enabled           bool      `json:"enabled"`
	BaseLatencyMs     int       `json:"base_latency_ms"`   // deterministic floor
	JitterMs          int       `json:"jitter_ms"`         // +/- random jitter
	ErrorRate         float64   `json:"error_rate"`        // 0..1, returns 5xx
	TimeoutRate       float64   `json:"timeout_rate"`      // 0..1, simulate very long processing (client likely times out)
	TimeoutMs         int       `json:"timeout_ms"`        // when timeout is chosen, sleep this long (unless ctx cancelled)
	NightBlockEnabled bool      `json:"night_block_enabled"`
	NightBlockWindow  TimeOfDay `json:"night_block_window"`
}

type Config struct {
	Lines map[LineID]*LineConfig `json:"lines"`
}

func parseClock(s string) (int, int) {
	if len(s) != 5 || s[2] != ':' { // naive check
		return 0, 0
	}
	h := int(s[0]-'0')*10 + int(s[1]-'0')
	m := int(s[3]-'0')*10 + int(s[4]-'0')
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0
	}
	return h, m
}

func inWindow(now time.Time, w TimeOfDay) bool {
	h1, m1 := parseClock(w.Start)
	h2, m2 := parseClock(w.End)
	start := time.Date(now.Year(), now.Month(), now.Day(), h1, m1, 0, 0, now.Location())
	end := time.Date(now.Year(), now.Month(), now.Day(), h2, m2, 0, 0, now.Location())

	if !end.After(start) { // crosses midnight
		// window is [start, 24h) U [0, end)
		return now.Equal(start) || now.After(start) || now.Before(end)
	}
	return (now.Equal(start) || now.After(start)) && now.Before(end)
}

type RNG struct {
	mu  sync.Mutex
	rng *rand.Rand
}

func newRNG() *RNG {
	return &RNG{rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

func (r *RNG) Float64() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rng.Float64()
}

func (r *RNG) Intn(n int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rng.Intn(n)
}

// defaultConfig provides sensible defaults reflecting typical behavior differences.
func defaultConfig() *Config {
	return &Config{Lines: map[LineID]*LineConfig{
		LineOuterUnified: {
			Name:              "外网统一 (outer-unified)",
			Enabled:           true,
			BaseLatencyMs:     220,
			JitterMs:          80,
			ErrorRate:         0.02,
			TimeoutRate:       0.01,
			TimeoutMs:         15000,
			NightBlockEnabled: false,
			NightBlockWindow:  TimeOfDay{Start: "00:30", End: "06:00"},
		},
		LineInnerUnified: {
			Name:              "内网统一 (inner-unified)",
			Enabled:           true,
			BaseLatencyMs:     80,
			JitterMs:          40,
			ErrorRate:         0.02,
			TimeoutRate:       0.01,
			TimeoutMs:         15000,
			NightBlockEnabled: true, // 内网夜间关闭
			NightBlockWindow:  TimeOfDay{Start: "00:30", End: "06:00"},
		},
		LineOuterZf: {
			Name:              "外网正方 (outer-zf)",
			Enabled:           true,
			BaseLatencyMs:     420,
			JitterMs:          120,
			ErrorRate:         0.08,
			TimeoutRate:       0.03,
			TimeoutMs:         20000,
			NightBlockEnabled: false,
			NightBlockWindow:  TimeOfDay{Start: "00:30", End: "06:00"},
		},
		LineInnerZf: {
			Name:              "内网正方 (inner-zf)",
			Enabled:           true,
			BaseLatencyMs:     160,
			JitterMs:          60,
			ErrorRate:         0.05,
			TimeoutRate:       0.02,
			TimeoutMs:         20000,
			NightBlockEnabled: true, // 内网夜间关闭
			NightBlockWindow:  TimeOfDay{Start: "00:30", End: "06:00"},
		},
	}}
}
