# BUILDING
#
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./ 

RUN go mod download 

COPY . .

RUN GCO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /main cmd/api/main.go 

FROM alpine:latest

RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /main .

COPY --from=builder /app/db/migrations ./db/migrations

USER appuser
EXPOSE 8080

CMD ["./main"]


