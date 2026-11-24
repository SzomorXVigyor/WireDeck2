FROM golang:1.26.2-alpine3.23 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY pkg/ pkg/
COPY services/webvnc-proxy/ services/webvnc-proxy/

ARG APP_VERSION=""
RUN VER="${APP_VERSION:-v$(date -u +%y.%m-%d%H%M)}" && \
    CGO_ENABLED=0 GOOS=linux go build \
      -ldflags="-s -w -X wiredeck/pkg/version.Version=${VER}" \
      -o /bin/webvnc-proxy ./services/webvnc-proxy

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /bin/webvnc-proxy /webvnc-proxy
EXPOSE 8080 8081
ENTRYPOINT ["/webvnc-proxy"]
