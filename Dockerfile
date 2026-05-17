FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /pcmi-api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -o /pcmi-worker ./cmd/worker

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

# API
COPY --from=builder /pcmi-api .
# Worker (opzionale per ora)
COPY --from=builder /pcmi-worker .

EXPOSE 8000
CMD ["./pcmi-api"]