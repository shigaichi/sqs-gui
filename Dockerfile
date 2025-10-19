
FROM node:25.0.0-bookworm-slim AS frontend-builder
WORKDIR /app
ENV PNPM_VERSION=10.18.3
RUN npm install -g pnpm@${PNPM_VERSION}
COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY assets ./assets
COPY public ./public
COPY templates ./templates
COPY tsconfig.json vite.config.ts ./
COPY biome.json global.d.ts ./
RUN pnpm run build

FROM golang:1.24.7-bookworm AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY templates ./templates
COPY dist.go ./
COPY --from=frontend-builder /app/dist ./dist
RUN go build -ldflags="-s -w" -trimpath -o /app/sqs-gui ./cmd/main.go

FROM gcr.io/distroless/base-debian12:nonroot-297ad518419983da72771db0d744bfa65d64a93e AS runtime
WORKDIR /app
COPY --from=go-builder /app/sqs-gui /usr/local/bin/sqs-gui
USER nonroot
EXPOSE 8080
ENV DEV_MODE=false
ENTRYPOINT ["/usr/local/bin/sqs-gui"]
