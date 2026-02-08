# build stage
FROM golang:1.25.6-alpine AS build

WORKDIR /app

# Go modules
COPY go.mod go.sum ./
RUN go mod download

# Source
COPY . .

# Build binaire gateway
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /gateway ./cmd/gateway

# runtime stage
FROM alpine:3.19

WORKDIR /app

# Copier le binaire et les ressources nécessaires
COPY --from=build /gateway /app/gateway
COPY web /app/web

ENV APP_PORT=8080

EXPOSE 8080

CMD ["/app/gateway"]
