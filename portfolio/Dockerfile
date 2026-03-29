# Build stage 1: Frontend
FROM node:20-alpine AS frontend-build
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm install
COPY frontend/ ./
RUN npm run build

# Build stage 2: Go backend
FROM golang:1.23-alpine AS backend-build
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY main.go ./
COPY content/ ./content/
COPY --from=frontend-build /app/static ./static/
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

# Runtime stage
FROM alpine:3.20
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=backend-build /app/server .
COPY --from=backend-build /app/content ./content/
EXPOSE 8080
ENV PORT=8080
CMD ["./server"]
