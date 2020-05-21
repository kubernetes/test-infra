FROM ubuntu:16.04

RUN apt-get -qq update && \
    apt-get -qq install -y --no-install-recommends \
    apt-transport-https \
    bash-completion \
    curl \
    g++ \
    git \
    openjdk-8-jdk \
    patch \
    python \
    ssh \
    unzip \
    xz-utils \
    zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*

# Install bazelisk on the path as Bazel so we don't need to explicitly keep Bazel up to date.
ENV BAZELISK_VERSION="1.4.0"
RUN curl -LSs -o /usr/local/bin/bazel "https://github.com/bazelbuild/bazelisk/releases/download/v${BAZEL_VERSION}/bazelisk-linux-amd64" \
    && chmod +x /usr/local/bin/bazel

RUN mkdir /bazel_cache && chmod -R 777 /bazel_cache

# Download and install gosu, so we can drop privs after the container starts.
RUN curl -LSs -o /usr/local/bin/gosu -SL "https://github.com/tianon/gosu/releases/download/1.4/gosu-$(dpkg --print-architecture)" \
    && chmod +x /usr/local/bin/gosu

# Copy the entrypoint into the image and set it as the target.
COPY bin/ /usr/local/bin
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
