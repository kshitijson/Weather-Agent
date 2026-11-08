# ---- Build stage ----
FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o weather-agent .

# ---- Run stage ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
WORKDIR /app
COPY --from=builder /app/weather-agent .
USER app

ENV PORT=8080
EXPOSE 8080
CMD ["./weather-agent"]
