# Dockerfile for image used to run CI.

FROM alpine:3.7
RUN apk --no-cache add alpine-sdk git openssl-dev

RUN git clone https://github.com/google/jsonnet && \
    git  -C jsonnet checkout v0.12.1 && \
    make -C jsonnet LDFLAGS=-static

FROM prom/prometheus:v2.8.1

FROM circleci/golang:1.10.3-stretch
COPY --from=0 jsonnet/jsonnet /usr/bin
COPY --from=1 /bin/promtool /usr/bin
RUN go get github.com/jsonnet-bundler/jsonnet-bundler/cmd/jb
