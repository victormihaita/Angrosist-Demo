# syntax=docker/dockerfile:1

# ---- build stage ----------------------------------------------------------
FROM golang:1.25-alpine AS build
WORKDIR /src

# Cache deps first.
COPY go.mod go.sum ./
RUN go mod download

# Build the two binaries (server + migration runner) from the same module.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/server ./cmd/server \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/migrate ./cmd/migrate \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/worker ./cmd/worker

# ---- runtime stage --------------------------------------------------------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 app
WORKDIR /app

COPY --from=build /out/server /app/server
COPY --from=build /out/migrate /app/migrate
COPY --from=build /out/worker /app/worker
# SQL migrations are read at runtime by the migrate binary (cwd/migrations).
COPY --from=build /src/migrations /app/migrations

USER app
EXPOSE 8080

# Default: run the API server. The migrate service overrides CMD with /app/migrate.
CMD ["/app/server"]
