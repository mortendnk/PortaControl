# Build: docker build -f deploy/docker/Dockerfile -t portacontrol .
FROM node:22-alpine AS web
WORKDIR /src/server/web
COPY server/web/package.json server/web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY server/web/ ./
RUN npm run build

FROM golang:1.27-alpine AS build
ARG VERSION=docker
ARG COMMIT=none
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
COPY --from=web /src/server/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/mortendnk/portacontrol/server/internal/version.Version=${VERSION} -X github.com/mortendnk/portacontrol/server/internal/version.Commit=${COMMIT}" \
    -o /out/ ./cmd/portacontrold ./cmd/portacontrol-cli

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && adduser -D -H -u 10001 portacontrol && mkdir -p /data && chown portacontrol /data
COPY --from=build /out/ /usr/local/bin/
USER portacontrol
ENV PORTACONTROL_DATA_DIR=/data PORTACONTROL_LISTEN=:8080
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/usr/local/bin/portacontrold", "-config", "/data/config.yaml"]
