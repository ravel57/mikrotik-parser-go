# mikrotik-parser-go

## Env
```bash
export APP_HTTP_PORT=8080
export APP_PG_DSN='postgres://user:pass@localhost:5432/dbname?sslmode=disable'

export APP_MIKROTIK_ADDR='192.168.88.1:8728'
export APP_MIKROTIK_USER='admin'
export APP_MIKROTIK_PASSWORD='password'

export APP_IGNORE_VPN_LIST='ignoreVpn'
export APP_COLLECT_SECONDS=10
```

## DB
Apply `migrations/0001_init.up.sql` using psql (or any migration tool).

## Run
```bash
go mod tidy
go get github.com/go-routeros/routeros@latest
go run ./cmd/server
```

## API
- GET `/api/v1/src?srcIp=...`
- GET `/api/v1/dns?find=...`
- POST `/api/v1/dns?dns=domain1,domain2&enabled=true|false`
