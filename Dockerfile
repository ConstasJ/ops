# Build stage
FROM golang:alpine AS builder

WORKDIR /src

RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /bin/ops .

# Final runtime image
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /bin/ops /usr/local/bin/ops

# 创建标准配置与运行数据目录
RUN mkdir -p /etc/ops /var/lib/ops /var/backups/ops /var/lib/ops/reports

VOLUME ["/etc/ops", "/var/lib/ops", "/var/backups/ops", "/var/run/docker.sock"]

ENTRYPOINT ["/usr/local/bin/ops"]
CMD ["status"]
