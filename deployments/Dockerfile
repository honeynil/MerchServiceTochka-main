FROM golang:1.24 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o merch-service ./cmd/server

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/merch-service .
COPY --from=builder /app/migrations ./migrations
COPY .env .
EXPOSE 8080
CMD ["./merch-service"]