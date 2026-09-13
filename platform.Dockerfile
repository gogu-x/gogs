# ---- build ----
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /platform ./platform

# ---- runtime ----
FROM alpine:3.20
RUN addgroup -S app && adduser -S -G app app
COPY --from=builder /platform /platform
USER app
EXPOSE 7000 7001
ENTRYPOINT ["/platform"]
