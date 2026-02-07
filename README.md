# mikrotik-parser-go

## Env
```bash
export APP_HTTP_PORT=8080
export APP_SQLITE_DSN='file:..\..\mikrotik_parser.sqlite?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)'

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


``` 
sudo docker build -t mikrotik-parser-go . && \
  sudo docker run --name mikrotik-parser-go \
  -d --restart unless-stopped \
  --env APP_HTTP_PORT=8080 \
  --env APP_PG_DSN='file:..\..\mikrotik_parser.sqlite?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)' \
  --env APP_MIKROTIK_ADDR='192.168.88.1:8728' \
  --env APP_MIKROTIK_USER='admin' \
  --env APP_MIKROTIK_PASSWORD='password' \
  --env APP_IGNORE_VPN_LIST='ignoreVpn' \
  --env APP_COLLECT_SECONDS=10 \
  -p 8080:8080 mikrotik-parser-go
```
