FROM golang:1.26.2-alpine3.23 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY pkg/ pkg/
COPY services/core-iam/ services/core-iam/

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/core-iam ./services/core-iam

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /bin/core-iam /core-iam
EXPOSE 8080 8081
ENTRYPOINT ["/core-iam"]
