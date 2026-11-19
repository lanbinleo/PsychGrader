# Stage 1: Build React + Tailwind v4 Frontend
FROM node:22-alpine AS frontend-builder
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2: Build Go + Gin Backend (pure-Go SQLite, CGO_ENABLED=0)
FROM golang:1.23-alpine AS backend-builder
WORKDIR /src/backend
ENV CGO_ENABLED=0
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN go build -trimpath -ldflags="-s -w" -o /out/psychgrader .

# Stage 3: Minimal Production Runtime with poppler-utils (pdftotext)
FROM alpine:3.21
RUN apk add --no-cache ca-certificates poppler-utils tzdata && \
    addgroup -g 1000 -S app && adduser -u 1000 -S -G app app
ENV TZ=Asia/Shanghai \
    HOST=0.0.0.0 \
    PORT=8090 \
    GIN_MODE=release \
    DB_PATH=/app/data/psychgrader.db \
    STATIC_DIR=/app/static

WORKDIR /app
COPY --from=backend-builder /out/psychgrader /app/psychgrader
COPY --from=frontend-builder /src/frontend/dist /app/static
RUN mkdir -p /app/data && chown -R 1000:1000 /app

USER 1000:1000
VOLUME ["/app/data"]
EXPOSE 8090
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8090/api/health >/dev/null || exit 1

ENTRYPOINT ["/app/psychgrader"]
