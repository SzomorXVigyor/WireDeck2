FROM golang:1.26.2-alpine3.23 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY pkg/ pkg/
COPY services/webview-engine/ services/webview-engine/

ARG APP_VERSION=""
RUN VER="${APP_VERSION:-v$(date -u +%y.%m-%d%H%M)}" && \
    CGO_ENABLED=0 GOOS=linux go build \
      -ldflags="-s -w -X wiredeck/pkg/version.Version=${VER}" \
      -o /bin/webview-engine ./services/webview-engine

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /bin/webview-engine /webview-engine
EXPOSE 8080 8081
ENTRYPOINT ["/webview-engine"]
