#############
# Binary tools
#############

FROM golang:1.13.8 as binary_tools_context

# Pinned versions of stuff
ENV GOIMPORTS_VERSION=379209517ffe
ENV GOLANGCI_LINT_VERSION=v1.23.1
ENV HADOLINT_VERSION=v1.17.2
ENV HELM_VERSION=v2.16.1
ENV KUBECTL_VERSION=v1.15.0
ENV KUBEBUILDER_VERSION=2.2.0
ENV OPERATOR_SDK_VERSION=v0.12.0
ENV SHELLCHECK_VERSION=v0.7.0

ENV GO111MODULE=on
ENV GOPROXY=https://proxy.golang.org

WORKDIR /tmp
ENV GOPATH=/tmp/go

ENV OUTDIR=/out
RUN mkdir -p ${OUTDIR}/usr/bin
RUN mkdir -p ${OUTDIR}/usr/local
RUN mkdir -p ${OUTDIR}/usr/lib

# Update distro and install dependencies
# hadolint ignore=DL3008
RUN apt-get update && apt-get install -y --no-install-recommends \
    apt-transport-https \
    build-essential \
    ca-certificates \
    curl \
    gnupg2 \
    software-properties-common \
    unzip \
    xz-utils

# Build and install a bunch of Go tools
RUN go get -ldflags="-s -w" golang.org/x/tools/cmd/goimports@${GOIMPORTS_VERSION}
RUN go get -ldflags="-s -w" github.com/golangci/golangci-lint/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}

# Hadolint linter
ADD https://github.com/hadolint/hadolint/releases/download/${HADOLINT_VERSION}/hadolint-Linux-x86_64 ${OUTDIR}/usr/bin/hadolint
RUN chmod 555 ${OUTDIR}/usr/bin/hadolint

# Helm
ADD https://storage.googleapis.com/kubernetes-helm/helm-${HELM_VERSION}-linux-amd64.tar.gz /tmp
RUN tar -xf /tmp/helm-${HELM_VERSION}-linux-amd64.tar.gz -C /tmp
RUN mv /tmp/linux-amd64/helm ${OUTDIR}/usr/bin

# Kubectl
ADD https://storage.googleapis.com/kubernetes-release/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl ${OUTDIR}/usr/bin/kubectl
RUN chmod 555 ${OUTDIR}/usr/bin/kubectl

# kubebuilder
ADD https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KUBEBUILDER_VERSION}/kubebuilder_${KUBEBUILDER_VERSION}_linux_amd64.tar.gz /tmp
RUN mkdir -p ${OUTDIR}/usr/local/kubebuilder
RUN tar -xf /tmp/kubebuilder_${KUBEBUILDER_VERSION}_linux_amd64.tar.gz --strip-components=1 -C ${OUTDIR}/usr/local/kubebuilder

# operator-sdk
ADD https://github.com/operator-framework/operator-sdk/releases/download/${OPERATOR_SDK_VERSION}/operator-sdk-${OPERATOR_SDK_VERSION}-x86_64-linux-gnu ${OUTDIR}/usr/bin/operator-sdk
RUN chmod 555 ${OUTDIR}/usr/bin/operator-sdk

# ShellCheck linter
ADD https://storage.googleapis.com/shellcheck/shellcheck-${SHELLCHECK_VERSION}.linux.x86_64.tar.xz /tmp
RUN tar -xJf /tmp/shellcheck-${SHELLCHECK_VERSION}.linux.x86_64.tar.xz -C /tmp
RUN mv /tmp/shellcheck-${SHELLCHECK_VERSION}/shellcheck ${OUTDIR}/usr/bin

# gcloud SDK
ENV CLOUDSDK_PYTHON_SITEPACKAGES=1
ENV CLOUDSDK_CORE_DISABLE_PROMPTS=1
ENV CLOUDSDK_INSTALL_DIR=${OUTDIR}/usr/lib
RUN curl https://sdk.cloud.google.com > /tmp/install_gcloud_sdk
RUN chmod +x /tmp/install_gcloud_sdk && bash /tmp/install_gcloud_sdk

# Cleanup stuff we don't need in the final image
RUN rm -fr /usr/local/go/doc
RUN rm -fr /usr/local/go/test
RUN rm -fr /usr/local/go/api
RUN rm -fr /usr/local/go/bin/godoc
RUN rm -fr /usr/local/go/bin/gofmt

#############
# Nodejs
#############

FROM ubuntu:bionic as nodejs_tools_context

# Pinned versions of stuff we pull in
ENV MARKDOWN_SPELLCHECK_VERSION=v1.3.1
ENV NODEJS_VERSION=12.8.0

ADD https://nodejs.org/download/release/v${NODEJS_VERSION}/node-v${NODEJS_VERSION}-linux-x64.tar.gz /tmp
RUN tar -xzf /tmp/node-v${NODEJS_VERSION}-linux-x64.tar.gz --strip-components=1 -C /usr/local

ADD https://nodejs.org/download/release/v${NODEJS_VERSION}/node-v${NODEJS_VERSION}-headers.tar.gz /tmp
RUN tar -xzf /tmp/node-v${NODEJS_VERSION}-headers.tar.gz --strip-components=1 -C /usr/local

RUN npm init -y
RUN npm install --production --global \
    markdown-spellcheck@"${MARKDOWN_SPELLCHECK_VERSION}" \

# Clean up stuff we don't need in the final image
RUN rm -rf /usr/local/sbin
RUN rm -rf /usr/local/share

#############
# Ruby
#############

FROM ubuntu:bionic as ruby_tools_context

# Pinned versions of stuff we pull in
ENV AWESOMEBOT_VERSION=1.19.1
ENV HTMLPROOFER_VERSION=3.12.0
ENV LICENSEE_VERSION=9.11.0
ENV MDL_VERSION=0.5.0
ENV RUBY_VERSION=2.6.5

# hadolint ignore=DL3008
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    cmake \
    libssl-dev \
    pkg-config \
    zlib1g-dev

ADD https://cache.ruby-lang.org/pub/ruby/2.6/ruby-${RUBY_VERSION}.tar.gz /tmp
WORKDIR /tmp
RUN tar -xzf ruby-${RUBY_VERSION}.tar.gz
WORKDIR /tmp/ruby-${RUBY_VERSION}
RUN ./configure
RUN make
RUN make install

# Install verification tools
RUN gem install --no-wrappers --no-document mdl -v ${MDL_VERSION}
RUN gem install --no-wrappers --no-document html-proofer -v ${HTMLPROOFER_VERSION}
RUN gem install --no-wrappers --no-document awesome_bot -v ${AWESOMEBOT_VERSION}
RUN gem install --no-wrappers --no-document licensee -v ${LICENSEE_VERSION}

##############
# Python
##############

FROM ubuntu:bionic as python_context

# Pinned versions of stuff we pull in
ENV AUTOPEP8_VERSION=1.4.4
ENV YAMLLINT_VERSION=1.17.0

# hadolint ignore=DL3008
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    libc-dev \
    pkg-config \
    python3.7 \
    python3-distutils

# installing pip in this manner to get pip3.7 and not pip3.6
RUN curl https://bootstrap.pypa.io/get-pip.py -o get-pip.py && python3.7 get-pip.py

# Install Python stuff
# hadolint ignore=DL3013
RUN pip3 install --no-binary :all: autopep8==${AUTOPEP8_VERSION}
RUN pip3 install yamllint==${YAMLLINT_VERSION}

#############
# Base OS
#############

FROM ubuntu:bionic as base_os_context

# required for binary tools: ca-certificates, gcc, libc-dev, git, iptables, libltdl7
# required for general build: make, wget, curl
# required for ruby: libcurl4-openssl-dev
# required for python: python3, pkg-config
# hadolint ignore=DL3008
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    gcc \
    git \
    iptables \
    libltdl7 \
    libc-dev \
    libcurl4-openssl-dev \
    make \
    pkg-config \
    python3 \
    ssh \
    wget

# Clean up stuff we don't need in the final image
RUN rm -rf /var/lib/apt/lists/*
RUN rm -fr /usr/share/perl
RUN rm -fr /usr/share/perl5
RUN rm -fr /usr/share/python
RUN rm -fr /usr/share/bash-completion
RUN rm -fr /usr/share/bug
RUN rm -fr /usr/share/doc
RUN rm -fr /usr/share/dh-python
RUN rm -fr /usr/share/locale
RUN rm -fr /usr/share/man
RUN rm -fr /tmp/*

#############
# Final image
#############

# Prepare final output image
FROM scratch as build_tools

# General
ENV HOME=/home
ENV LANG=C.UTF-8

# Go support
ENV GO111MODULE=on
ENV GOPROXY=https://proxy.golang.org
ENV GOSUMDB=https://sum.golang.org
ENV GOROOT=/usr/local/go
ENV GOPATH=/home/go
ENV GOBIN=/home/gobin
ENV GOCACHE=/home/gocache
ENV PATH=/usr/lib/google-cloud-sdk/bin:/usr/local/kubebuilder/bin:/usr/local/go/bin:/home/go/bin:/home/gobin:$PATH

# Ruby suppport
ENV RUBYOPT="-KU -E utf-8:utf-8"

# Create the file system
COPY --from=base_os_context / /
COPY --from=binary_tools_context /out/ /
COPY --from=binary_tools_context /usr/local/go /usr/local/go
COPY --from=binary_tools_context /usr/lib/google-cloud-sdk /usr/lib/google-cloud-sdk
COPY --from=ruby_tools_context /usr/local/bin /usr/local/bin
COPY --from=ruby_tools_context /usr/local/lib /usr/local/lib
COPY --from=nodejs_tools_context /usr/local /usr/local
COPY --from=nodejs_tools_context /node_modules /node_modules
COPY --from=python_context /usr/local/bin /usr/local/bin
COPY --from=python_context /usr/local/lib /usr/local/lib

RUN mkdir -p /home/go && \
    mkdir -p /home/gobin && \
    mkdir -p /home/gocache && \
    mkdir -p /home/.kube && \
    mkdir -p /home/.helm

RUN chmod 777 /home/go && \
    chmod 777 /home/gobin && \
    chmod 777 /home/gocache && \
    chmod 777 /home/.kube && \
    chmod 777 /home/.helm && \

WORKDIR /home
