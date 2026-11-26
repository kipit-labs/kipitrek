# syntax=docker/dockerfile:1

# Build stages run on the builder's native platform and cross-compile,
# so multi-arch images don't need emulation.
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src/web
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
COPY VERSION /src/VERSION
RUN pnpm build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS api
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY web/embed.go web/
COPY --from=web /src/web/dist web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/kipitrek ./cmd/kipitrek

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=api /out/kipitrek /kipitrek
EXPOSE 8080
ENTRYPOINT ["/kipitrek"]
