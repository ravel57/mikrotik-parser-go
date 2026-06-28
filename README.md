# mikrotik-parser-go

## Env
```bash
export APP_HTTP_PORT=8080
export APP_SQLITE_DSN='file:.\mikrotik_parser.sqlite?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)'

export APP_MIKROTIK_ADDR='192.168.88.1:8728'
export APP_MIKROTIK_USER='admin'
export APP_MIKROTIK_PASSWORD='password'

export APP_IGNORE_VPN_LIST='ignoreVpn'
export APP_IGNORE_LAN_TO_VPN_LIST='ignoreLanToVpn'
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
  -e APP_HTTP_PORT=8080 \
  -e APP_SQLITE_DSN='file:./mikrotik_parser.sqlite?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)' \
  -e APP_MIKROTIK_ADDR='192.168.88.1:8728' \
  -e APP_MIKROTIK_USER='admin' \
  -e APP_MIKROTIK_PASSWORD='password' \
  -e APP_IGNORE_VPN_LIST='ignoreVpn' \
  -e APP_IGNORE_LAN_TO_VPN_LIST='ignoreLanToVpn' \
  -e APP_COLLECT_SECONDS=10 \
  -p 8080:8080 mikrotik-parser-go
```

### Build for arm v7
```
sudo docker buildx create --use --name mbuilder 2>/dev/null || true
sudo docker buildx use mbuilder
sudo docker buildx prune -af
sudo docker buildx build \
 --no-cache \
 --pull \
 --platform linux/arm/v7 \
 -t mikapp:latest \
 --load .
sudo docker save -o ./mikapp-armv7.tar mikapp:latest
sudo skopeo copy --format v2s2 \
  docker-archive:./mikapp-armv7.tar \
  docker-archive:./mikapp-armv7-docker.tar:mikapp:latest
```

### Build for arm 64
```
sudo docker buildx create --use --name mbuilder 2>/dev/null || true
sudo docker buildx use mbuilder
sudo docker buildx prune -af
sudo docker buildx build \
 --no-cache \
 --pull \
 --platform linux/arm64 \
 -t mikapp:latest \
 --load .
sudo docker save -o ./mikapp-arm64.tar mikapp:latest
sudo skopeo copy --format v2s2 \
  oci-archive:./mikapp-arm64.tar \
  docker-archive:./mikapp-arm64-docker.tar:mikapp:latest
```