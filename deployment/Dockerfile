ARG BUILDER_BASE=code-base-golang-api-builder-base:latest
ARG RUNTIME_BASE=code-base-golang-api-runtime-base:latest

FROM --platform=$BUILDPLATFORM ${BUILDER_BASE} AS builder

ARG TARGETOS=linux
ARG TARGETARCH

WORKDIR /src

COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /out/api ./cmd/api

FROM ${RUNTIME_BASE} AS runtime

ARG APP_VERSION=0.1.0
ARG GIT_HASH=dev

ENV APP_VERSION=${APP_VERSION}
ENV GIT_HASH=${GIT_HASH}

WORKDIR /app

COPY --from=builder /out/api /app/api
COPY --from=builder /src/migrations /app/migrations

USER app

EXPOSE 8080

CMD ["/app/api"]
