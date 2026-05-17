# ---- build ----
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /game ./game

# ---- runtime ----
FROM alpine:3.20
COPY --from=builder /game /game
EXPOSE 9001
ENTRYPOINT ["/game"]
