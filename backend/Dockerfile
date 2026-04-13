# ── Сборка ───────────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /app

RUN apk --no-cache add git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/server ./cmd/server

# ── Production ───────────────────────────────────────────────────────────────
FROM alpine:3.20 AS production

RUN apk --no-cache add ca-certificates tzdata postgresql-client
WORKDIR /app

COPY --from=builder /bin/server .
COPY migrations/ ./migrations/
COPY entrypoint.sh .
RUN chmod +x entrypoint.sh

EXPOSE 8080
ENTRYPOINT ["./entrypoint.sh"]

# ── Development (hot reload) ─────────────────────────────────────────────────
FROM golang:1.22-alpine AS development

WORKDIR /app

RUN go install github.com/air-verse/air@latest

COPY go.mod go.sum ./
RUN go mod download

EXPOSE 8080
CMD ["air"]
