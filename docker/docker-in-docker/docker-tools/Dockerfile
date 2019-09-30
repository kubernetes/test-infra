FROM ubuntu:xenial

# Installing
COPY docker-tools/install.sh /tmp/tools/
RUN chmod -R +x /tmp/tools/install.sh
RUN /tmp/tools/install.sh && rm -rf /tmp/tools

# Docker in Docker settings
VOLUME /var/lib/docker
EXPOSE 2375

ENV PATH /usr/local/go/bin:/home/go/bin:/usr/lib/google-cloud-sdk/bin:${PATH}

# COPY entrypoint to start docker
COPY docker-tools/prow-runner.sh /usr/local/bin/entrypoint
RUN chmod +rx /usr/local/bin/entrypoint

# Set CI variable which can be checked by test scripts to verify
# if running in the continuous integration environment.
ENV CI prow

# Go support
ENV GO111MODULE=on
ENV GOPROXY=https://proxy.golang.org
ENV GOSUMDB=https://sum.golang.org
ENV GOROOT=/usr/local/go
ENV GOPATH=/home/go
ENV GOCACHE=/home/gocache
ENV PATH=/usr/lib/google-cloud-sdk/bin:/usr/local/go/bin:/home/go/bin:$PATH

ENTRYPOINT ["entrypoint"]
