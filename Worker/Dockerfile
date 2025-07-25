FROM golang:1.24.2-alpine AS builder

WORKDIR /app
COPY . .
RUN go build -o main .


FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/main .
RUN chmod +x main
EXPOSE 8081

CMD ["./main"]
