FROM golang:1.24.2-alpine AS builder

RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o supervisor .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/supervisor .
EXPOSE 8080

CMD ["./supervisor"]
