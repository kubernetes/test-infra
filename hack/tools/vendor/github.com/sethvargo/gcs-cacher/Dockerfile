FROM alpine AS builder

FROM scratch
COPY gcs-cacher /bin/gcs-cacher
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["/bin/gcs-cacher"]
