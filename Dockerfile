# syntax=docker/dockerfile:1

# ---- Build stage ----
FROM --platform=linux/amd64 golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /awspim .

# ---- Runtime stage ----
FROM --platform=linux/amd64 gcr.io/distroless/static-debian12

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /awspim /awspim

# Config can be mounted at /etc/awspim/config.yaml via a ConfigMap/Secret volume,
# or supplied entirely through environment variables (see config.yaml.example).
WORKDIR /

USER nonroot:nonroot

ENTRYPOINT ["/awspim"]
