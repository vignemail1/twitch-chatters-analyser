FROM golang:1.25-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /analysis ./cmd/analysis

FROM alpine:3.19

WORKDIR /app

COPY --from=build /analysis /app/analysis

ENV APP_PORT=8083

CMD ["/app/analysis"]
