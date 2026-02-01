# ---------- build ----------
FROM --platform=$BUILDPLATFORM golang:1.25.6-alpine AS build
WORKDIR /app
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---------- run ----------
FROM alpine:3.23
WORKDIR /app
RUN apk add --no-cache ca-certificates && update-ca-certificates
COPY --from=build /out/server /app/server
# если у вас фронт лежит на диске, а не embed, раскомментируйте:
# COPY web/dist /app/web/dist
EXPOSE 8080
ENTRYPOINT ["/app/server"]
