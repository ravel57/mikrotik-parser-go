# ---------- build frontend ----------
FROM node:22-alpine AS nodejs
RUN apk add --no-cache git
WORKDIR /usr/src/node
RUN git clone https://github.com/ravel57/mikrotik_parser.git
WORKDIR /usr/src/node/mikrotik_parser
RUN ["yarn", "install"]
RUN ["yarn", "build"]

# ---------- build ----------
FROM --platform=$BUILDPLATFORM golang:1.25.6-alpine AS build
WORKDIR /app
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=nodejs /usr/src/node/mikrotik_parser/dist /app/web/dist
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---------- run ----------
FROM alpine:3.23
WORKDIR /app
RUN apk add --no-cache ca-certificates && update-ca-certificates
COPY --from=build /out/server /app/server
COPY --from=nodejs /usr/src/node/mikrotik_parser/dist /app/web/dist
EXPOSE 8080
ENTRYPOINT ["/app/server"]
