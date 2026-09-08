# ---- build ----
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /battle ./battle

# ---- runtime ----
FROM alpine:3.20
COPY --from=builder /battle /battle
EXPOSE 10901
ENTRYPOINT ["/battle"]
