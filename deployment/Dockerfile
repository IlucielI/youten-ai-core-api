FROM code-base-golang-api-base:latest AS builder

ARG TARGETOS=linux
ARG TARGETARCH

WORKDIR /src

COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /out/api ./cmd/api

FROM alpine:3.20 AS runtime

ARG APP_VERSION=0.1.0
ARG GIT_HASH=dev

ENV APP_VERSION=${APP_VERSION}
ENV GIT_HASH=${GIT_HASH}

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata ffmpeg wget python3 quickjs \
    && wget https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -O /usr/local/bin/yt-dlp \
    && chmod a+rx /usr/local/bin/yt-dlp \
    && echo '--js-runtimes quickjs:qjs' > /etc/yt-dlp.conf \
    && addgroup -S app \
    && adduser -S app -G app

COPY --from=builder /out/api /app/api
COPY --from=builder /src/migrations /app/migrations

USER app

EXPOSE 8080

CMD ["/app/api"]
