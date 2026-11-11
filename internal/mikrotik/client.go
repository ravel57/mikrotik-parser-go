package mikrotik

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-routeros/routeros/v3"
)

type Client struct {
	addr string
	user string
	pass string

	mu sync.Mutex
	c  *routeros.Client
}

func New(addr, user, pass string) *Client {
	return &Client{addr: addr, user: user, pass: pass}
}

func (m *Client) Connect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.c != nil {
		return nil
	}
	if m.addr == "" {
		return errors.New("mikrotik addr is empty")
	}

	// DialContext: connects and logs in using context.
	c, err := routeros.DialContext(ctx, m.addr, m.user, m.pass)
	if err != nil {
		return err
	}
	m.c = c
	return nil
}

func (m *Client) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.c == nil {
		return nil
	}
	err := m.c.Close()
	m.c = nil
	return err
}

func (m *Client) run(ctx context.Context, sentence ...string) (*routeros.Reply, error) {
	// Чтобы не зависать навсегда, ограничим RunContext локальным таймаутом.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 6*time.Second)
		defer cancel()
	}

	if err := m.Connect(ctx); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	return m.c.RunContext(ctx, sentence...)
}

// replyToMaps конвертирует []*proto.Sentence -> []map[string]string через Sentence.Map.
// Копируем map, чтобы вызывающий код не зависел от внутренностей библиотеки.
func replyToMaps(r *routeros.Reply) []map[string]string {
	if r == nil || len(r.Re) == 0 {
		return nil
	}

	out := make([]map[string]string, 0, len(r.Re))
	for _, s := range r.Re {
		if s == nil {
			continue
		}
		if s.Map == nil {
			out = append(out, map[string]string{})
			continue
		}
		mm := make(map[string]string, len(s.Map))
		for k, v := range s.Map {
			mm[k] = v
		}
		out = append(out, mm)
	}
	return out
}

func (m *Client) DNSCache(ctx context.Context) ([]map[string]string, error) {
	r, err := m.run(ctx, "/ip/dns/cache/print")
	if err != nil {
		return nil, err
	}
	return replyToMaps(r), nil
}

func (m *Client) DHCPLeases(ctx context.Context) ([]map[string]string, error) {
	r, err := m.run(ctx, "/ip/dhcp-server/lease/print")
	if err != nil {
		return nil, err
	}
	return replyToMaps(r), nil
}

func (m *Client) FirewallConnections(ctx context.Context) ([]map[string]string, error) {
	r, err := m.run(ctx, "/ip/firewall/connection/print")
	if err != nil {
		return nil, err
	}
	return replyToMaps(r), nil
}

func (m *Client) AddressListIgnoreVPN(ctx context.Context, listName string) ([]map[string]string, error) {
	r, err := m.run(ctx,
		"/ip/firewall/address-list/print",
		"?list="+listName,
	)
	if err != nil {
		return nil, err
	}
	return replyToMaps(r), nil
}

func (m *Client) AddressListSetDisabled(ctx context.Context, id string, disabled bool) error {
	val := "no"
	if disabled {
		val = "yes"
	}
	_, err := m.run(ctx,
		"/ip/firewall/address-list/set",
		"=disabled="+val,
		"=.id="+id,
	)
	return err
}

func (m *Client) AddressListAdd(ctx context.Context, listName, address string) error {
	_, err := m.run(ctx,
		"/ip/firewall/address-list/add",
		"=list="+listName,
		"=address="+address,
	)
	return err
}

type SystemResource struct {
	UptimeSeconds  float64
	CPULoadPercent float64
	TotalMemory    int64
	FreeMemory     int64
}

func (m *Client) SystemResource(ctx context.Context) (SystemResource, error) {
	r, err := m.run(ctx, "/system/resource/print")
	if err != nil {
		return SystemResource{}, err
	}
	rows := replyToMaps(r)
	if len(rows) == 0 {
		return SystemResource{}, errors.New("empty /system/resource/print reply")
	}
	row := rows[0]

	var info SystemResource
	info.UptimeSeconds = parseRouterOSUptime(row["uptime"])
	info.CPULoadPercent, _ = strconv.ParseFloat(row["cpu-load"], 64)
	info.TotalMemory, _ = strconv.ParseInt(row["total-memory"], 10, 64)
	info.FreeMemory, _ = strconv.ParseInt(row["free-memory"], 10, 64)
	return info, nil
}

var routerOSUptimePattern = regexp.MustCompile(`(\d+)(w|d|h|m|s)`)

// parseRouterOSUptime turns a RouterOS uptime string such as "4w3d12h5m33s"
// into a number of seconds.
func parseRouterOSUptime(value string) float64 {
	var total float64
	for _, match := range routerOSUptimePattern.FindAllStringSubmatch(value, -1) {
		n, err := strconv.ParseFloat(match[1], 64)
		if err != nil {
			continue
		}
		switch match[2] {
		case "w":
			total += n * 7 * 24 * 3600
		case "d":
			total += n * 24 * 3600
		case "h":
			total += n * 3600
		case "m":
			total += n * 60
		case "s":
			total += n
		}
	}
	return total
}

// InterfaceTraffic returns the current rx/tx bitrate (bits per second) of the
// given interface, using RouterOS's own live rate measurement.
func (m *Client) InterfaceTraffic(ctx context.Context, iface string) (rxBitsPerSec, txBitsPerSec float64, err error) {
	iface = strings.TrimSpace(iface)
	if iface == "" {
		return 0, 0, errors.New("interface name is not configured")
	}
	r, runErr := m.run(ctx, "/interface/monitor-traffic", "=interface="+iface, "=once=")
	if runErr != nil {
		return 0, 0, runErr
	}
	rows := replyToMaps(r)
	if len(rows) == 0 {
		return 0, 0, fmt.Errorf("empty /interface/monitor-traffic reply for %q", iface)
	}
	row := rows[0]
	rxBitsPerSec, _ = strconv.ParseFloat(row["rx-bits-per-second"], 64)
	txBitsPerSec, _ = strconv.ParseFloat(row["tx-bits-per-second"], 64)
	return rxBitsPerSec, txBitsPerSec, nil
}

// DefaultRouteInterface returns the interface used by the active default
// route (0.0.0.0/0), so callers don't need to hard-code a WAN interface
// name, which varies a lot between RouterOS setups.
func (m *Client) DefaultRouteInterface(ctx context.Context) (string, error) {
	r, err := m.run(ctx, "/ip/route/print", "?dst-address=0.0.0.0/0")
	if err != nil {
		return "", err
	}
	rows := replyToMaps(r)

	isActive := func(row map[string]string) bool {
		v := strings.ToLower(strings.TrimSpace(row["active"]))
		return v == "true" || v == "yes"
	}

	// Prefer a route RouterOS itself marks active; fall back to the first
	// row with an interface if the "active" flag isn't present in the reply.
	for _, row := range rows {
		if isActive(row) {
			if iface := strings.TrimSpace(row["interface"]); iface != "" {
				return iface, nil
			}
		}
	}
	for _, row := range rows {
		if iface := strings.TrimSpace(row["interface"]); iface != "" {
			return iface, nil
		}
	}
	return "", errors.New("no default route found")
}

func SplitDomainsCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
