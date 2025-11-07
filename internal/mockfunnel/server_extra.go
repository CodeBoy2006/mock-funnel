package mockfunnel

import (
	"encoding/json"
	"net/http"
)

// Add metrics snapshot handler (polled by UI)
func (s *Server) initMetricsHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/metrics/snapshot", s.handleMetricsSnapshot)
}

type series struct {
	Sec         []int64 `json:"sec"`
	RPS         []int   `json:"rps"`
	LatencyAvg  []int   `json:"latency_avg"`
	Success     []int   `json:"success"`
	Errors      []int   `json:"errors"`
	Timeouts    []int   `json:"timeouts"`
}

type snapshot struct {
	Series map[LineID]series `json:"series"`
	Totals map[LineID]*LineMetrics `json:"totals"`
}

func (s *Server) handleMetricsSnapshot(w http.ResponseWriter, r *http.Request) {
	s.metricsMu.RLock()
	defer s.metricsMu.RUnlock()
	resp := snapshot{Series: make(map[LineID]series), Totals: make(map[LineID]*LineMetrics)}
	for id, m := range s.metrics {
		secs, rps, avg, succ, errs, timeouts := m.Win.snapshot()
		resp.Series[id] = series{Sec: secs, RPS: rps, LatencyAvg: avg, Success: succ, Errors: errs, Timeouts: timeouts}
		resp.Totals[id] = &LineMetrics{
			Requests: m.Requests,
			Success:  m.Success,
			Errors:   m.Errors,
			Timeouts: m.Timeouts,
			P50: m.P50, P95: m.P95, P99: m.P99,
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}
