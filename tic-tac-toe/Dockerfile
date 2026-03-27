FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY main.go index.html ./
RUN CGO_ENABLED=0 go build -o /tic-tac-toe .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /tic-tac-toe /tic-tac-toe
EXPOSE 8080
ENTRYPOINT ["/tic-tac-toe"]
