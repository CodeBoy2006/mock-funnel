package mockfunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	webui "mock-funnel/web"
)

// 解析模板（从 embed.FS 读取）
var indexTmpl = template.Must(template.ParseFS(webui.Assets, "templates/index.html"))

// 静态资源子 FS
var staticFS, _ = fs.Sub(webui.Assets, "static")

type Server struct {
	cfgMu sync.RWMutex
	cfg   *Config

	rng *RNG

	metricsMu sync.RWMutex
	metrics   map[LineID]*LineMetrics
}

func NewServer() *Server {
	s := &Server{
		cfg:     defaultConfig(),
		rng:     newRNG(),
		metrics: make(map[LineID]*LineMetrics),
	}
	for _, id := range AllLines {
		s.metrics[id] = newLineMetrics()
	}
	return s
}

func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()

	// metrics endpoints
	s.initMetricsHandlers(mux)

	// 静态资源与首页
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	mux.HandleFunc("/", s.handleIndex)

	// Admin API
	mux.HandleFunc("/admin/config", s.handleGetConfig)   // GET
	mux.HandleFunc("/admin/reset", s.handleResetMetrics) // POST
	mux.HandleFunc("/admin/line/", s.handleLineConfig)   // GET/POST for /admin/line/{line}

	// Public mock API (no auth)
	// Supported: /{line}/api/ping, /{line}/api/schedule, /{line}/api/grades
	mux.HandleFunc("/outer-unified/", s.dispatchAPI)
	mux.HandleFunc("/inner-unified/", s.dispatchAPI)
	mux.HandleFunc("/outer-zf/", s.dispatchAPI)
	mux.HandleFunc("/inner-zf/", s.dispatchAPI)

	return logRequests(mux)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if err := indexTmpl.Execute(w, nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(s.cfg)
}

func (s *Server) handleResetMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	s.metricsMu.Lock()
	for _, m := range s.metrics {
		m.reset()
	}
	s.metricsMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLineConfig(w http.ResponseWriter, r *http.Request) {
	// path: /admin/line/{line}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 3 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	lineStr := parts[2]
	id := LineID(lineStr)

	s.cfgMu.RLock()
	_, ok := s.cfg.Lines[id]
	s.cfgMu.RUnlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("unknown line"))
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.cfgMu.RLock()
		defer s.cfgMu.RUnlock()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(s.cfg.Lines[id])
		return
	case http.MethodPost:
		var req LineConfig
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("invalid json"))
			return
		}
		s.cfgMu.Lock()
		// keep name if empty to avoid accidental erase
		if req.Name == "" {
			req.Name = s.cfg.Lines[id].Name
		}
		s.cfg.Lines[id] = &req
		s.cfgMu.Unlock()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(req)
		return
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
}

func (s *Server) dispatchAPI(w http.ResponseWriter, r *http.Request) {
	// Expected /{line}/api/{resource}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}
	lineStr := parts[0]
	if parts[1] != "api" {
		http.NotFound(w, r)
		return
	}
	resource := parts[2]

	id := LineID(lineStr)
	s.cfgMu.RLock()
	cfg, ok := s.cfg.Lines[id]
	s.cfgMu.RUnlock()
	if !ok {
		http.Error(w, "unknown line", http.StatusNotFound)
		return
	}

	// simulate processing for this line
	status, payload, latencyMs := s.simulate(r.Context(), id, cfg, resource)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Mock-Line", string(id))
	w.Header().Set("X-Mock-Latency-Ms", fmt.Sprintf("%d", latencyMs))
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

type payload struct {
	OK       bool        `json:"ok"`
	Line     string      `json:"line"`
	Resource string      `json:"resource"`
	Now      string      `json:"now"`
	Latency  int         `json:"latency_ms"`
	Data     interface{} `json:"data,omitempty"`
	Error    string      `json:"error,omitempty"`
}

func (s *Server) simulate(ctx context.Context, id LineID, cfg *LineConfig, resource string) (status int, pl payload, latencyMs int) {
	start := time.Now()
	defer func() {
		latencyMs = int(time.Since(start).Milliseconds())
	}()

	// Night block
	if cfg.NightBlockEnabled && inWindow(time.Now(), cfg.NightBlockWindow) {
		pl = payload{OK: false, Line: string(id), Resource: resource, Now: time.Now().Format(time.RFC3339), Error: "nightly window blocked"}
		s.record(id, start, int(time.Since(start).Milliseconds()), false, false)
		return http.StatusServiceUnavailable, pl, int(time.Since(start).Milliseconds())
	}

	// Enabled?
	if !cfg.Enabled {
		pl = payload{OK: false, Line: string(id), Resource: resource, Now: time.Now().Format(time.RFC3339), Error: "line disabled"}
		s.record(id, start, int(time.Since(start).Milliseconds()), false, false)
		return http.StatusServiceUnavailable, pl, int(time.Since(start).Milliseconds())
	}

	// Base + jitter
	delay := cfg.BaseLatencyMs
	if cfg.JitterMs > 0 {
		j := s.rng.Intn(cfg.JitterMs*2+1) - cfg.JitterMs // [-jitter, +jitter]
		if delay+j >= 0 {
			delay += j
		}
	}

	// maybe timeout (long processing) before anything else
	if cfg.TimeoutRate > 0 && s.rng.Float64() < cfg.TimeoutRate {
		// Sleep for TimeoutMs or until context cancelled
		select {
		case <-time.After(time.Duration(cfg.TimeoutMs) * time.Millisecond):
			// proceed to emit 504 to indicate server-side timeout
			pl = payload{OK: false, Line: string(id), Resource: resource, Now: time.Now().Format(time.RFC3339), Error: "simulated timeout"}
			s.record(id, start, int(time.Since(start).Milliseconds()), false, true)
			return http.StatusGatewayTimeout, pl, int(time.Since(start).Milliseconds())
		case <-ctx.Done():
			// client gave up; we still record as timeout
			pl = payload{OK: false, Line: string(id), Resource: resource, Now: time.Now().Format(time.RFC3339), Error: "client cancelled"}
			s.record(id, start, int(time.Since(start).Milliseconds()), false, true)
			return http.StatusGatewayTimeout, pl, int(time.Since(start).Milliseconds())
		}
	}

	// normal processing delay
	select {
	case <-time.After(time.Duration(delay) * time.Millisecond):
		// proceed
	case <-ctx.Done():
		pl = payload{OK: false, Line: string(id), Resource: resource, Now: time.Now().Format(time.RFC3339), Error: "client cancelled"}
		s.record(id, start, int(time.Since(start).Milliseconds()), false, true)
		return http.StatusGatewayTimeout, pl, int(time.Since(start).Milliseconds())
	}

	// maybe error
	if cfg.ErrorRate > 0 && s.rng.Float64() < cfg.ErrorRate {
		pl = payload{OK: false, Line: string(id), Resource: resource, Now: time.Now().Format(time.RFC3339), Error: "simulated upstream error"}
		s.record(id, start, int(time.Since(start).Milliseconds()), false, false)
		return http.StatusBadGateway, pl, int(time.Since(start).Milliseconds())
	}

	// success
	data := map[string]any{
		"message":  "mock data",
		"resource": resource,
		"hint":     "response body shape is stable, feel free to ignore data for LB testing",
	}
	pl = payload{OK: true, Line: string(id), Resource: resource, Now: time.Now().Format(time.RFC3339), Latency: int(time.Since(start).Milliseconds()), Data: data}
	s.record(id, start, int(time.Since(start).Milliseconds()), true, false)
	return http.StatusOK, pl, int(time.Since(start).Milliseconds())
}

func (s *Server) record(id LineID, start time.Time, latencyMs int, ok bool, isTimeout bool) {
	s.metricsMu.RLock()
	m := s.metrics[id]
	s.metricsMu.RUnlock()
	if m == nil {
		return
	}
	m.addSample(start, latencyMs, ok, isTimeout)
}