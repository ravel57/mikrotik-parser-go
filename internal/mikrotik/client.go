package mikrotik

import (
	"context"
	"errors"
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
