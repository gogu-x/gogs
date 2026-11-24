FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN mkdir /out && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ ./battle ./game ./gate ./platform
FROM alpine:3.20
COPY --from=builder /out/ /usr/local/bin/
COPY gogs-entrypoint.sh /usr/local/bin/gogs-entrypoint
RUN chmod 755 /usr/local/bin/gogs-entrypoint && addgroup -S app && adduser -S -G app app
USER app
ENTRYPOINT ["/usr/local/bin/gogs-entrypoint"]
