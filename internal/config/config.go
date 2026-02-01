package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPPort string

	SqliteDSN string

	MikrotikAddr string // host:port, usually 8728
	MikrotikUser string
	MikrotikPass string

	IgnoreVPNListName      string
	IgnoreLanToVpnListName string

	CollectInterval time.Duration

	StaticDir string
}

func Load() Config {
	interval := 10 * time.Second
	if v := os.Getenv("APP_COLLECT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			interval = time.Duration(n) * time.Second
		}
	}

	ignoreList := os.Getenv("APP_IGNORE_VPN_LIST")
	if ignoreList == "" {
		ignoreList = "ignoreVpn"
	}

	ignoreLanToVpn := os.Getenv("APP_IGNORE_LAN_TO_VPN_LIST")
	if ignoreLanToVpn == "" {
		ignoreLanToVpn = "ignoreLanToVpn"
	}

	port := os.Getenv("APP_HTTP_PORT")
	if port == "" {
		port = "8080"
	}

	staticDir := os.Getenv("APP_STATIC_DIR")
	if staticDir == "" {
		staticDir = "web/dist"
	}

	dsn := os.Getenv("APP_SQLITE_DSN")
	if dsn == "" {
		dsn = os.Getenv("APP_PG_DSN")
	}

	return Config{
		HTTPPort: port,

		SqliteDSN: dsn,

		MikrotikAddr: os.Getenv("APP_MIKROTIK_ADDR"),
		MikrotikUser: os.Getenv("APP_MIKROTIK_USER"),
		MikrotikPass: os.Getenv("APP_MIKROTIK_PASSWORD"),

		IgnoreVPNListName:      ignoreList,
		IgnoreLanToVpnListName: ignoreLanToVpn,

		CollectInterval: interval,

		StaticDir: staticDir,
	}
}
