# build stage
FROM golang:1.25.6-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /worker ./cmd/worker

# runtime
FROM alpine:3.19

WORKDIR /app

COPY --from=build /worker /app/worker

ENV JOB_POLL_INTERVAL=2

CMD ["/app/worker"]
