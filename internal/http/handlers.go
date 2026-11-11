package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mikrotik-parser-go/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

type Handler struct {
	connections            *service.ConnectionsService
	collect                *service.CollectService
	stats                  *service.StatsService
	staticDir              string
	ignoreVPNListName      string
	ignoreLanToVpnListName string
	statsPushInterval      time.Duration
}

func NewHandler(
	connections *service.ConnectionsService,
	collect *service.CollectService,
	stats *service.StatsService,
	staticDir string,
	ignoreVPNListName string,
	ignoreLanToVpnListName string,
	statsPushInterval time.Duration,
) *Handler {
	return &Handler{
		connections:            connections,
		collect:                collect,
		stats:                  stats,
		staticDir:              staticDir,
		ignoreVPNListName:      ignoreVPNListName,
		ignoreLanToVpnListName: ignoreLanToVpnListName,
		statsPushInterval:      statsPushInterval,
	}
}

var statsWSUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()

	// API
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/config", h.getConfig)
		r.Get("/src", h.getSrc)                            // ?srcIp=
		r.Get("/dns", h.getByDNS)                          // ?find=
		r.Post("/dns", h.postDNS)                          // ?dns=&enabled=
		r.Get("/ignore-lan-to-vpn", h.getIgnoreLanToVpn)   // ?find=
		r.Post("/ignore-lan-to-vpn", h.postIgnoreLanToVpn) // JSON {ip, enabled}
		r.Get("/stats", h.getStats)
		r.Get("/stats/ws", h.getStatsWS)
	})

	// Frontend (как в Spring: "/" -> index, + /js/**, /css/**, /favicon.ico)
	if h.staticDir != "" && dirExists(h.staticDir) {
		fs := http.FileServer(http.Dir(h.staticDir))

		// точечные роуты как в MvcConfig
		r.Get("/", h.serveIndex)
		r.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(h.staticDir, "favicon.ico"))
		})
		r.Handle("/js/*", fs)
		r.Handle("/css/*", fs)

		// если во фронте есть другие ассеты (images/fonts и т.п.)
		r.Handle("/assets/*", fs)

		// SPA fallback: любой неизвестный путь (кроме /api/*) -> index.html
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}
			h.serveIndex(w, r)
		})
	}

	return r
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func (h *Handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join(h.staticDir, "index.html"))
}

type configResp struct {
	IgnoreVPNListName      string `json:"ignoreVPNListName"`
	IgnoreLanToVpnListName string `json:"ignoreLanToVpnListName"`
}

func (h *Handler) getConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, configResp{
		IgnoreVPNListName:      h.ignoreVPNListName,
		IgnoreLanToVpnListName: h.ignoreLanToVpnListName,
	})
}

func (h *Handler) getSrc(w http.ResponseWriter, r *http.Request) {
	srcIP := r.URL.Query().Get("srcIp")
	res, err := h.connections.GetBySrc(r.Context(), srcIP)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, res)
}

func (h *Handler) getByDNS(w http.ResponseWriter, r *http.Request) {
	find := r.URL.Query().Get("find")
	res, err := h.collect.GetByDNS(r.Context(), find)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, res)
}

func (h *Handler) postDNS(w http.ResponseWriter, r *http.Request) {
	dns := r.URL.Query().Get("dns")
	enabled := r.URL.Query().Get("enabled") == "true"

	if err := h.connections.PostDnsToIgnoreList(r.Context(), dns, enabled); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

type ignoreLanToVpnReq struct {
	IP       string `json:"ip"`
	Enabled  bool   `json:"enabled"`
	HostName string `json:"hostName,omitempty"`
}

func (h *Handler) getIgnoreLanToVpn(w http.ResponseWriter, r *http.Request) {
	find := r.URL.Query().Get("find")
	items, err := h.connections.GetIgnoreLanToVpn(r.Context(), find)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, items)
}

func (h *Handler) getStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.stats.Get(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// getStatsWS pushes a fresh Stats snapshot to the client right away and then
// again on every tick, until the socket is closed, so the UI can stay live
// without polling.
func (h *Handler) getStatsWS(w http.ResponseWriter, r *http.Request) {
	conn, err := statsWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ctx := r.Context()
	interval := h.statsPushInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}

	sendStats := func() bool {
		stats, err := h.stats.Get(ctx)
		if err != nil {
			return true
		}
		return conn.WriteJSON(stats) == nil
	}

	if !sendStats() {
		return
	}

	// WriteJSON isn't safe to call concurrently with reads, but we only need
	// the read loop to notice when the client goes away (close frame, pings,
	// or a dropped connection).
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-closed:
			return
		case <-ticker.C:
			if !sendStats() {
				return
			}
		}
	}
}

func (h *Handler) postIgnoreLanToVpn(w http.ResponseWriter, r *http.Request) {
	// поддержка и query, и JSON
	ip := r.URL.Query().Get("ip")
	enabledStr := r.URL.Query().Get("enabled")

	var req ignoreLanToVpnReq
	if ip == "" {
		_ = json.NewDecoder(r.Body).Decode(&req)
		ip = req.IP
		if enabledStr == "" {
			if req.Enabled {
				enabledStr = "true"
			} else {
				enabledStr = "false"
			}
		}
	}

	enabled := enabledStr == "true" || enabledStr == "1"
	if err := h.connections.PostIpToIgnoreLanToVpn(r.Context(), ip, enabled); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
