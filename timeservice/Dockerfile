FROM golang:1.23-alpine AS build
WORKDIR /app
COPY go.mod ./
COPY main.go ./
RUN go build -o timeservice .

FROM alpine:3.20
COPY --from=build /app/timeservice /timeservice
EXPOSE 8080
ENTRYPOINT ["/timeservice"]
