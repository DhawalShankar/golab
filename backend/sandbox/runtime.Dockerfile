FROM alpine:3.22

RUN apk add --no-cache coreutils

RUN addgroup -S -g 10001 sandbox \
    && adduser -S -D -H -u 10001 -G sandbox sandbox

USER 10001:10001

WORKDIR /workspace