FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build

# Copier go.mod et go.sum pour le cache des dépendances
COPY go.mod go.sum ./
RUN go mod download

# Copier le code source
COPY . .

# Compiler le binaire
RUN CGO_ENABLED=0 GOOS=linux go build -o twitch-api ./cmd/twitch-api

# Image finale légère
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copier le binaire depuis le builder
COPY --from=builder /build/twitch-api .

# Port par défaut
EXPOSE 8081

# Lancer le service
CMD ["./twitch-api"]
