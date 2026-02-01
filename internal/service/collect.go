package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"mikrotik-parser-go/internal/storage"
)

type CollectService struct {
	connections *ConnectionsService
	repo        *storage.Sqlite
	interval    time.Duration
}

func NewCollectService(connections *ConnectionsService, repo *storage.Sqlite, interval time.Duration) *CollectService {
	return &CollectService{connections: connections, repo: repo, interval: interval}
}

func (c *CollectService) Run(ctx context.Context) {
	t := time.NewTicker(c.interval)
	defer t.Stop()

	type key struct {
		ip  string
		dns string
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			conns, err := c.connections.GetConnections(ctx)
			if err != nil {
				continue
			}

			// DNS -> count
			mDNS := map[string]int64{}
			// (IP,DNS) -> count
			mDst := map[key]int64{}

			for _, cn := range conns {
				dns := strings.TrimSpace(cn.DstDNS)
				if dns == "" {
					continue
				}
				ip := strings.TrimSpace(cn.DstIP)
				if ip == "" {
					continue
				}
				mDNS[dns]++
				mDst[key{ip: ip, dns: dns}]++
			}

			domainCounts := make([]storage.DomainCount, 0, len(mDNS))
			for dns, cnt := range mDNS {
				domainCounts = append(domainCounts, storage.DomainCount{DstDNS: dns, Count: cnt})
			}

			dstCounts := make([]storage.DstCount, 0, len(mDst))
			for k, cnt := range mDst {
				dstCounts = append(dstCounts, storage.DstCount{DstIP: k.ip, DstDNS: k.dns, Count: cnt})
			}

			_ = c.repo.UpsertDomainCounts(ctx, domainCounts)
			_ = c.repo.UpsertDstCounts(ctx, dstCounts)
		}
	}
}

func (c *CollectService) GetByDNS(ctx context.Context, name string) ([]map[string]any, error) {
	rows, err := c.repo.FindDstCountsLike(ctx, name) // читаем dst_conn_counts
	if err != nil {
		return nil, err
	}

	// группируем по домену, чтобы фронт получил dnsConnections
	type agg struct {
		dns      string
		sum      int64
		updated  string
		ipsSeen  map[string]struct{}
		ipsSlice []map[string]any
	}

	byDNS := map[string]*agg{}
	for _, r := range rows {
		dns := strings.TrimSpace(r.DstDNS)
		ip := strings.TrimSpace(r.DstIP)
		if dns == "" || ip == "" {
			continue
		}

		a := byDNS[dns]
		if a == nil {
			a = &agg{
				dns:     dns,
				ipsSeen: map[string]struct{}{},
			}
			byDNS[dns] = a
		}

		a.sum += r.Count
		if r.UpdatedAt > a.updated {
			a.updated = r.UpdatedAt
		}

		if _, ok := a.ipsSeen[ip]; !ok {
			a.ipsSeen[ip] = struct{}{}
			a.ipsSlice = append(a.ipsSlice, map[string]any{
				"dstIP":  ip,
				"dstDNS": dns,
			})
		}
	}

	// в массив
	out := make([]map[string]any, 0, len(byDNS))
	for _, a := range byDNS {
		ignore, _ := c.connections.IsIgnoreVPN(ctx, a.dns)

		out = append(out, map[string]any{
			"dstDns":            a.dns,
			"activeConnections": a.sum,
			"dnsConnections":    a.ipsSlice,
			"isIgnoreVpn":       ignore,
			"updatedAt":         a.updated,
		})
	}

	// сортировка как раньше: по activeConnections desc
	sort.Slice(out, func(i, j int) bool {
		return out[i]["activeConnections"].(int64) > out[j]["activeConnections"].(int64)
	})

	return out, nil
}
