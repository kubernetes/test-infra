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

# Install the requested version of bazel and stick it in the right place so that the tools/bazel script doesn't re-download the same version.
ENV BAZEL_VERSION="0.27.0"
RUN export BAZEL_INSTALL_DIR="/home/user/.bazel_installations/${BAZEL_VERSION}" \
    && curl -LSs -o /tmp/bazel.deb https://github.com/bazelbuild/bazel/releases/download/${BAZEL_VERSION}/bazel_${BAZEL_VERSION}-linux-x86_64.deb \
    && dpkg -i /tmp/bazel.deb \
    && rm /tmp/bazel.deb \
    && mkdir -p ${BAZEL_INSTALL_DIR}/bin \
    && ln -s /usr/bin/bazel-real ${BAZEL_INSTALL_DIR}/bin/bazel

RUN mkdir /bazel_cache && chmod -R 777 /bazel_cache

# Download and install gosu, so we can drop privs after the container starts.
RUN curl -LSs -o /usr/local/bin/gosu -SL "https://github.com/tianon/gosu/releases/download/1.4/gosu-$(dpkg --print-architecture)" \
    && chmod +x /usr/local/bin/gosu

# Copy the entrypoint into the image and set it as the target.
COPY bin/ /usr/local/bin
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
