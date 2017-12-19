FROM gcr.io/google_containers/ubuntu-slim:0.1

ENV GIT_SYNC_DEST /git
VOLUME ["/git"]

RUN apt-get update && \
  apt-get install -y git ca-certificates --no-install-recommends && \
  apt-get clean -y && \
  rm -rf /var/lib/apt/lists/*

COPY git-sync /git-sync

RUN mkdir /nonexistent && chmod 777 /nonexistent

ENTRYPOINT ["/git-sync"]
