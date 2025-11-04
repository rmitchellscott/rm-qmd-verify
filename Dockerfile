FROM rust:1.83-alpine AS qmldiff-builder

RUN apk add --no-cache git musl-dev

WORKDIR /build
RUN git clone --depth 1 https://github.com/asivery/qmldiff.git && \
    cd qmldiff && \
    cargo build --release

FROM node:24-alpine AS frontend-builder

WORKDIR /build

COPY ui/package.json ui/package-lock.json ./

RUN npm ci

COPY ui/ ./

RUN npm run build

FROM golang:1.23-alpine AS backend-builder

WORKDIR /build

RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./

RUN go mod download

COPY . .

COPY --from=frontend-builder /build/dist ./ui/dist

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s \
    -X github.com/rmitchellscott/rm-qmd-verify/internal/version.Version=${VERSION} \
    -X github.com/rmitchellscott/rm-qmd-verify/internal/version.Commit=${COMMIT} \
    -X github.com/rmitchellscott/rm-qmd-verify/internal/version.BuildTime=${BUILD_TIME}" \
    -o rm-qmd-verify \
    .

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=qmldiff-builder /build/qmldiff/target/release/qmldiff /app/bin/qmldiff

COPY --from=backend-builder /build/rm-qmd-verify /app/rm-qmd-verify

RUN mkdir -p /app/hashtables

ENV PORT=8080 \
    HASHTAB_DIR=/app/hashtables \
    QMLDIFF_BINARY=/app/bin/qmldiff

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/ || exit 1

CMD ["/app/rm-qmd-verify"]
