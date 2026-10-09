FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./ 

RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /main ./cmd/api

FROM alpine:3.22

RUN apk add --no-cache postgresql15-client ffmpeg font-dejavu \
    && addgroup -S appgroup && adduser -S appuser -G appgroup

RUN mkdir -p /data/recordings && chown 1000:1000 /data/recordings

WORKDIR /app

COPY --from=builder /main .

COPY --from=builder /app/db/migrations ./db/migrations

USER appuser
EXPOSE 8080

CMD ["./main"]


