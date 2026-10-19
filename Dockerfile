# 1.26: github.com/lxc/incus/v7 needs >= 1.25.11, and the official image pins GOTOOLCHAIN=local (no patch download).
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder
WORKDIR /app

# Module manifests first, for layer caching.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -o /app/worker .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/worker .
ENTRYPOINT ["./worker"]
