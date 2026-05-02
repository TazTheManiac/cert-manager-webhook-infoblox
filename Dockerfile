# ─── Stage 1: resolve dependencies ───────────────────────────────────────────
FROM golang:1.26-alpine AS deps

LABEL org.opencontainers.image.source="https://github.com/tazthemaniac/cert-manager-webhook-infoblox"

RUN apk add --no-cache git

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

# ─── Stage 2: build ───────────────────────────────────────────────────────────
FROM deps AS build

COPY . .
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux go build -o /webhook -ldflags '-w -extldflags "-static"' .

# ─── Stage 3: minimal runtime image ──────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.source="https://github.com/tazthemaniac/cert-manager-webhook-infoblox"
LABEL org.opencontainers.image.description="cert-manager DNS01 webhook for Infoblox WAPI"
LABEL org.opencontainers.image.licenses="Apache-2.0"

COPY --from=build /webhook /webhook

ENTRYPOINT ["/webhook", "-v=4"]
