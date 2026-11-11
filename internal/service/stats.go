package service

import (
	"context"
	"strings"
	"time"

	"mikrotik-parser-go/internal/mikrotik"
	"mikrotik-parser-go/internal/storage"
)

const topDomainsLimit = 10

type ListStats struct {
	Total   int `json:"total"`
	Enabled int `json:"enabled"`
}

type TopDomainStat struct {
	DstDNS            string `json:"dstDns"`
	ActiveConnections int64  `json:"activeConnections"`
	UpdatedAt         string `json:"updatedAt"`
}

type ConnectionsStats struct {
	TrackedDomains      int64           `json:"trackedDomains"`
	TrackedDestinations int64           `json:"trackedDestinations"`
	ActiveConnections   int64           `json:"activeConnections"`
	TopDomains          []TopDomainStat `json:"topDomains"`
}

type NetworkStats struct {
	DownloadBytesPerSec float64 `json:"downloadBytesPerSec"`
	UploadBytesPerSec   float64 `json:"uploadBytesPerSec"`
	UpdatedAt           string  `json:"updatedAt"`
}

type SystemStats struct {
	UptimeSeconds  float64 `json:"uptimeSeconds"`
	CPUUsedPercent float64 `json:"cpuUsedPercent"`
	MemUsedPercent float64 `json:"memUsedPercent"`
}

type Stats struct {
	Connections ConnectionsStats `json:"connections"`
	Network     NetworkStats     `json:"network"`
	System      SystemStats      `json:"system"`
	IgnoreVPN   ListStats        `json:"ignoreVpn"`
	IgnoreLAN   ListStats        `json:"ignoreLanToVpn"`
}

type StatsService struct {
	connections            *ConnectionsService
	mt                     *mikrotik.Client
	repo                   *storage.Sqlite
	ignoreVPNListName      string
	ignoreLanToVpnListName string
	statsInterface         string
}

func NewStatsService(
	connections *ConnectionsService,
	mt *mikrotik.Client,
	repo *storage.Sqlite,
	ignoreVPNListName string,
	ignoreLanToVpnListName string,
	statsInterface string,
) *StatsService {
	return &StatsService{
		connections:            connections,
		mt:                     mt,
		repo:                   repo,
		ignoreVPNListName:      ignoreVPNListName,
		ignoreLanToVpnListName: ignoreLanToVpnListName,
		statsInterface:         statsInterface,
	}
}

func (s *StatsService) Get(ctx context.Context) (Stats, error) {
	var out Stats

	out.IgnoreVPN = s.listStats(ctx, s.ignoreVPNListName)
	out.IgnoreLAN = s.listStats(ctx, s.ignoreLanToVpnListName)

	if summary, err := s.repo.DomainStatsSummary(ctx); err == nil {
		out.Connections.TrackedDomains = summary.TrackedDomains
		out.Connections.TrackedDestinations = summary.TrackedDestinations
		out.Connections.ActiveConnections = summary.ActiveConnections
	}
	if top, err := s.repo.TopDomainCounts(ctx, topDomainsLimit); err == nil {
		out.Connections.TopDomains = make([]TopDomainStat, 0, len(top))
		for _, d := range top {
			out.Connections.TopDomains = append(out.Connections.TopDomains, TopDomainStat{
				DstDNS:            d.DstDNS,
				ActiveConnections: d.Count,
				UpdatedAt:         d.UpdatedAt,
			})
		}
	}

	iface := strings.TrimSpace(s.statsInterface)
	if iface == "" {
		iface, _ = s.mt.DefaultRouteInterface(ctx)
	}
	if iface != "" {
		if rx, tx, err := s.mt.InterfaceTraffic(ctx, iface); err == nil {
			out.Network.DownloadBytesPerSec = rx / 8
			out.Network.UploadBytesPerSec = tx / 8
			out.Network.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		}
	}

	if info, err := s.mt.SystemResource(ctx); err == nil {
		out.System.UptimeSeconds = info.UptimeSeconds
		out.System.CPUUsedPercent = info.CPULoadPercent
		if info.TotalMemory > 0 {
			out.System.MemUsedPercent = 100 * float64(info.TotalMemory-info.FreeMemory) / float64(info.TotalMemory)
		}
	}

	return out, nil
}

func (s *StatsService) listStats(ctx context.Context, listName string) ListStats {
	rows, err := s.mt.AddressListIgnoreVPN(ctx, listName)
	if err != nil {
		return ListStats{}
	}
	stats := ListStats{Total: len(rows)}
	for _, r := range rows {
		d := strings.ToLower(strings.TrimSpace(r["disabled"]))
		if !(d == "true" || d == "yes") {
			stats.Enabled++
		}
	}
	return stats
}
