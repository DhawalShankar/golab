FROM golang:1.26-alpine

RUN apk add --no-cache coreutils

RUN addgroup -S -g 10001 sandbox \
    && adduser -S -D -H -u 10001 -G sandbox sandbox

RUN mkdir -p \
    /tmp/home \
    /tmp/go-cache \
    /tmp/go-mod-cache \
    && chown -R 10001:10001 /tmp

ENV GOTOOLCHAIN=local
ENV CGO_ENABLED=0
ENV GOPROXY=off
ENV GOSUMDB=off
ENV HOME=/tmp/home
ENV GOCACHE=/tmp/go-cache
ENV GOMODCACHE=/tmp/go-mod-cache

USER 10001:10001

WORKDIR /workspace