package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	"mikrotik-parser-go/internal/domain"
	"mikrotik-parser-go/internal/mikrotik"
)

type ConnectionsService struct {
	mt *mikrotik.Client

	ignoreVPNListName      string
	ignoreLanToVpnListName string
}

func NewConnectionsService(mt *mikrotik.Client, ignoreVPNListName, ignoreLanToVpnListName string) *ConnectionsService {
	return &ConnectionsService{
		mt:                     mt,
		ignoreVPNListName:      ignoreVPNListName,
		ignoreLanToVpnListName: ignoreLanToVpnListName,
	}
}

func stripPort(s string) string {
	if strings.Count(s, ":") == 1 {
		i := strings.LastIndex(s, ":")
		if i > 0 {
			if _, err := strconv.Atoi(s[i+1:]); err == nil {
				return s[:i]
			}
		}
	}
	return s
}

func (s *ConnectionsService) GetConnections(ctx context.Context) ([]domain.Connection, error) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	dnsRows, err := s.mt.DNSCache(ctx)
	if err != nil {
		return nil, err
	}
	leaseRows, err := s.mt.DHCPLeases(ctx)
	if err != nil {
		return nil, err
	}
	connRows, err := s.mt.FirewallConnections(ctx)
	if err != nil {
		return nil, err
	}

	dnsByIP := map[string]string{}
	for _, r := range dnsRows {
		ip := r["data"]
		if ip == "" {
			ip = r["address"]
		}
		name := r["name"]
		if name == "" {
			name = r["dns-name"]
		}
		if ip != "" && name != "" {
			dnsByIP[ip] = name
		}
	}

	hostByIP := map[string]string{}
	for _, r := range leaseRows {
		ip := r["active-address"]
		if ip == "" {
			ip = r["address"]
		}
		host := r["active-host-name"]
		if host == "" {
			host = r["host-name"]
		}
		if host == "" {
			host = r["comment"]
		}
		if ip != "" && host != "" {
			hostByIP[ip] = host
		}
	}

	out := make([]domain.Connection, 0, len(connRows))
	for _, r := range connRows {
		src := stripPort(r["src-address"])
		dst := stripPort(r["dst-address"])
		if src == "" || dst == "" {
			continue
		}
		out = append(out, domain.Connection{
			SrcIP:    src,
			DstIP:    dst,
			DstDNS:   dnsByIP[dst],
			HostName: hostByIP[src],
		})
	}
	return out, nil
}

func (s *ConnectionsService) GetBySrc(ctx context.Context, srcIP string) ([]domain.GroupedDnsConnection, error) {
	conns, err := s.GetConnections(ctx)
	if err != nil {
		return nil, err
	}

	group := map[string][]domain.DnsConnection{}
	for _, c := range conns {
		if c.SrcIP != srcIP {
			continue
		}
		key := c.DstDNS
		group[key] = append(group[key], domain.DnsConnection{DstIP: c.DstIP, DstDNS: c.DstDNS})
	}

	res := make([]domain.GroupedDnsConnection, 0, len(group))
	for dns, items := range group {
		ignore, _ := s.IsIgnoreVPN(ctx, dns)
		res = append(res, domain.GroupedDnsConnection{
			DstDNS:      dns,
			Items:       items,
			IsIgnoreVPN: ignore,
		})
	}
	return res, nil
}

func (s *ConnectionsService) PostDnsToIgnoreList(ctx context.Context, domains string, enabled bool) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	addresses, err := s.mt.AddressListIgnoreVPN(ctx, s.ignoreVPNListName)
	if err != nil {
		return err
	}

	byAddress := map[string]map[string]string{}
	for _, r := range addresses {
		if a := r["address"]; a != "" {
			byAddress[a] = r
		}
	}

	for _, domain := range mikrotik.SplitDomainsCSV(domains) {
		row := byAddress[domain]
		id := ""
		if row != nil {
			id = row[".id"]
		}

		if enabled {
			if id != "" {
				if err := s.mt.AddressListSetDisabled(ctx, id, false); err != nil {
					return err
				}
			} else {
				if err := s.mt.AddressListAdd(ctx, s.ignoreVPNListName, domain); err != nil {
					return err
				}
			}
		} else {
			if id != "" {
				if err := s.mt.AddressListSetDisabled(ctx, id, true); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *ConnectionsService) IsIgnoreVPN(ctx context.Context, dns string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	if strings.TrimSpace(dns) == "" {
		return false, nil
	}

	addresses, err := s.mt.AddressListIgnoreVPN(ctx, s.ignoreVPNListName)
	if err != nil {
		return false, err
	}

	enabledStatic := map[string]bool{}
	for _, r := range addresses {
		if r["dynamic"] == "true" {
			continue
		}
		if r["disabled"] == "true" {
			continue
		}
		if a := r["address"]; a != "" {
			enabledStatic[a] = true
		}
	}

	for _, d := range mikrotik.SplitDomainsCSV(dns) {
		if !enabledStatic[d] {
			return false, nil
		}
	}
	return true, nil
}

type IgnoreLanToVpnItem struct {
	IP       string `json:"ip"`
	HostName string `json:"hostName"`
	Enabled  bool   `json:"enabled"`
	Dynamic  bool   `json:"dynamic"`
}

func (s *ConnectionsService) GetIgnoreLanToVpn(ctx context.Context, find string) ([]IgnoreLanToVpnItem, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	// адрес-лист ignoreLanToVpn
	addresses, err := s.mt.AddressListIgnoreVPN(ctx, s.ignoreLanToVpnListName) // метод уже есть, просто другой listName
	if err != nil {
		return nil, err
	}

	// DHCP leases для матчинга IP -> active host name
	leaseRows, err := s.mt.DHCPLeases(ctx)
	if err != nil {
		return nil, err
	}

	hostByIP := map[string]string{}
	for _, r := range leaseRows {
		ip := r["active-address"]
		if ip == "" {
			ip = r["address"]
		}
		host := r["active-host-name"]
		if host == "" {
			host = r["host-name"]
		}
		if host == "" {
			host = r["comment"]
		}
		if ip != "" && host != "" {
			hostByIP[ip] = host
		}
	}

	find = strings.ToLower(strings.TrimSpace(find))

	out := make([]IgnoreLanToVpnItem, 0, len(addresses))
	for _, r := range addresses {
		ip := strings.TrimSpace(r["address"])
		if ip == "" {
			continue
		}

		host := hostByIP[ip]
		if host == "" {
			// fallback: если в address-list записан comment
			host = strings.TrimSpace(r["comment"])
		}

		d := strings.ToLower(strings.TrimSpace(r["disabled"]))
		enabled := !(d == "true" || d == "yes")
		dyn := r["dynamic"] == "true"

		// фильтр по ip/hostname (опционально)
		if find != "" {
			if !strings.Contains(strings.ToLower(ip), find) &&
				!strings.Contains(strings.ToLower(host), find) {
				continue
			}
		}

		out = append(out, IgnoreLanToVpnItem{
			IP:       ip,
			HostName: host,
			Enabled:  enabled,
			Dynamic:  dyn,
		})
	}
	return out, nil
}

func (s *ConnectionsService) PostIpToIgnoreLanToVpn(ctx context.Context, ip string, enabled bool) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	ip = strings.TrimSpace(ip)
	if ip == "" {
		return nil
	}

	addresses, err := s.mt.AddressListIgnoreVPN(ctx, s.ignoreLanToVpnListName)
	if err != nil {
		return err
	}

	var id string
	for _, r := range addresses {
		if strings.TrimSpace(r["address"]) == ip {
			id = r[".id"]
			break
		}
	}

	if enabled {
		if id != "" {
			return s.mt.AddressListSetDisabled(ctx, id, false)
		}
		return s.mt.AddressListAdd(ctx, s.ignoreLanToVpnListName, ip)
	}

	if id != "" {
		return s.mt.AddressListSetDisabled(ctx, id, true)
	}
	return nil
}
