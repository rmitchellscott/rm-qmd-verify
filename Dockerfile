FROM --platform=$BUILDPLATFORM tonistiigi/xx:1.7.0 AS xx

FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend-builder

WORKDIR /build

COPY ui/package.json ui/package-lock.json ./

RUN npm ci

COPY ui/ ./

RUN npm run build

FROM --platform=$BUILDPLATFORM rust:1-alpine AS qmldiff-builder

WORKDIR /build

RUN apk add --no-cache git musl-dev

# Clone upstream qmldiff repository
RUN git clone --depth 1 https://github.com/asivery/qmldiff.git qmldiff

# Build qmldiff CLI binary
WORKDIR /build/qmldiff
RUN cargo build --release --bin qmldiff

FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS backend-builder

COPY --from=xx / /

WORKDIR /build

RUN apk add --no-cache git ca-certificates tzdata clang lld musl-dev

COPY go.mod go.sum ./

RUN go mod download

COPY main.go ./
COPY internal/ ./internal/
COPY pkg/ ./pkg/

COPY --from=frontend-builder /build/dist ./ui/dist

ARG TARGETPLATFORM
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

# Build main server binary (no CGO needed - uses qmldiff CLI binary)
RUN CGO_ENABLED=0 xx-go build -trimpath \
    -ldflags="-w -s \
    -X github.com/rmitchellscott/rm-qmd-verify/internal/version.Version=${VERSION} \
    -X github.com/rmitchellscott/rm-qmd-verify/internal/version.Commit=${COMMIT} \
    -X github.com/rmitchellscott/rm-qmd-verify/internal/version.BuildTime=${BUILD_TIME}" \
    -o rm-qmd-verify \
    . && \
    xx-verify rm-qmd-verify

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata libgcc

WORKDIR /app

COPY --from=backend-builder /build/rm-qmd-verify /app/rm-qmd-verify
COPY --from=qmldiff-builder /build/qmldiff/target/release/qmldiff /app/qmldiff

RUN mkdir -p /app/hashtables /app/qml-trees

ENV PORT=8080 \
    HASHTAB_DIR=/app/hashtables \
    QML_TREE_DIR=/app/qml-trees \
    QMLDIFF_BINARY=/app/qmldiff

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/ || exit 1

CMD ["/app/rm-qmd-verify"]
