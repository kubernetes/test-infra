#############
# Binary tools
#############

# Use golang as the binary_tools_context
FROM golang:1.15.7 as binary_tools_context

# Pinned versions of stuff
ENV GOIMPORTS_VERSION=379209517ffe
ENV GOLANGCI_LINT_VERSION=v1.23.1
ENV HADOLINT_VERSION=v1.17.2
ENV HELM_VERSION=v3.4.1
ENV KUBECTL_VERSION=v1.16.0
ENV SHELLCHECK_VERSION=v0.7.0

ENV GO111MODULE=on
ENV GOPROXY=https://proxy.golang.org

WORKDIR /tmp
ENV GOPATH=/tmp/go

ENV OUTDIR=/out
RUN mkdir -p ${OUTDIR}/usr/bin
RUN mkdir -p ${OUTDIR}/usr/local

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
ADD https://get.helm.sh/helm-${HELM_VERSION}-linux-amd64.tar.gz /tmp
RUN tar -xf /tmp/helm-${HELM_VERSION}-linux-amd64.tar.gz -C /tmp
RUN mv /tmp/linux-amd64/helm ${OUTDIR}/usr/bin

# Kubectl
ADD https://storage.googleapis.com/kubernetes-release/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl ${OUTDIR}/usr/bin/kubectl
RUN chmod 555 ${OUTDIR}/usr/bin/kubectl

# ShellCheck linter
ADD https://github.com/koalaman/shellcheck/releases/download/${SHELLCHECK_VERSION}/shellcheck-${SHELLCHECK_VERSION}.linux.x86_64.tar.xz /tmp
RUN tar -xJf /tmp/shellcheck-${SHELLCHECK_VERSION}.linux.x86_64.tar.xz -C /tmp
RUN mv /tmp/shellcheck-${SHELLCHECK_VERSION}/shellcheck ${OUTDIR}/usr/bin

# Move go tools to OUTDIR
RUN mv /tmp/go/bin/* ${OUTDIR}/usr/bin

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
ENV SASS_LINT_VERSION=v1.13.1
ENV SASS_VERSION=v1.23.7
ENV TSLINT_VERSION=v5.20.0
ENV TYPESCRIPT_VERSION=v3.7.2

ADD https://nodejs.org/download/release/v${NODEJS_VERSION}/node-v${NODEJS_VERSION}-linux-x64.tar.gz /tmp
RUN tar -xzf /tmp/node-v${NODEJS_VERSION}-linux-x64.tar.gz --strip-components=1 -C /usr/local

ADD https://nodejs.org/download/release/v${NODEJS_VERSION}/node-v${NODEJS_VERSION}-headers.tar.gz /tmp
RUN tar -xzf /tmp/node-v${NODEJS_VERSION}-headers.tar.gz --strip-components=1 -C /usr/local

RUN npm install --production --global \
    sass@"${SASS_VERSION}" \
    sass-lint@"${SASS_LINT_VERSION}" \
    typescript@"${TYPESCRIPT_VERSION}" \
    tslint@"${TSLINT_VERSION}" \
    markdown-spellcheck@"${MARKDOWN_SPELLCHECK_VERSION}"

# Clean up stuff we don't need in the final image
RUN rm -rf /usr/local/sbin
RUN rm -rf /usr/local/share

#############
# Ruby
#############

FROM ubuntu:bionic as ruby_tools_context

# Pinned versions of stuff we pull in
ENV AWESOMEBOT_VERSION=1.19.1
ENV FPM_VERSION=1.11.0
ENV HTMLPROOFER_VERSION=3.12.0
ENV LICENSEE_VERSION=9.11.0
ENV MDL_VERSION=0.5.0

# hadolint ignore=DL3008
RUN apt-get update && apt-get -y dist-upgrade && apt-get install -y --no-install-recommends \
    apt-transport-https \
    ca-certificates \
    gnupg2 \
    software-properties-common \
    build-essential \
    zlib1g-dev \
    cmake \
    pkg-config \
    libssl-dev

RUN add-apt-repository -y ppa:brightbox/ruby-ng-experimental
# hadolint ignore=DL3008
RUN apt-get update && apt-get install -y --no-install-recommends \
    ruby2.6 \
    ruby2.6-dev

# Install verification tools
RUN gem install --no-wrappers --no-document mdl -v ${MDL_VERSION}
RUN gem install --no-wrappers --no-document html-proofer -v ${HTMLPROOFER_VERSION}
RUN gem install --no-wrappers --no-document awesome_bot -v ${AWESOMEBOT_VERSION}
RUN gem install --no-wrappers --no-document licensee -v ${LICENSEE_VERSION}
RUN gem install --no-wrappers --no-document fpm -v ${FPM_VERSION}

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

# hadolint ignore=DL3008
RUN apt-get update && apt-get -y --no-install-recommends install \
    apt-transport-https \
    build-essential \
    ca-certificates \
    gnupg-agent \
    lsb-release \
    software-properties-common \
    curl \
    git \
    ssh \
    make \
    python3.7 \
    wget \
    jq \
  && apt-get clean \
  && rm -rf /var/lib/apt/lists/*

# gcloud SDK
ENV CLOUDSDK_PYTHON_SITEPACKAGES=1
ENV CLOUDSDK_CORE_DISABLE_PROMPTS=1
ENV CLOUDSDK_INSTALL_DIR=/usr/lib
RUN curl https://sdk.cloud.google.com > /tmp/install_gcloud_sdk
# FIXME: remove always true suffix after gcloud-sdk installation on ppc64le&s390x is fixed.
RUN chmod +x /tmp/install_gcloud_sdk && bash /tmp/install_gcloud_sdk || true

# Clean up unused stuff for the final image
RUN rm -rf /var/lib/apt/lists/*
RUN rm -fr /usr/share/bash-completion
RUN rm -fr /usr/share/bug
RUN rm -fr /usr/share/doc
RUN rm -fr /usr/share/locale
RUN rm -fr /usr/share/man
RUN rm -fr /tmp/*


##############
# Final image
##############

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
ENV GOPATH=${HOME}/go
ENV GOBIN=${HOME}/gobin
ENV GOCACHE=${HOME}/gocache
ENV PATH=/usr/lib/google-cloud-sdk/bin:/usr/local/kubebuilder/bin:/usr/local/go/bin:${HOME}/go/bin:${HOME}/gobin:$PATH

# Docker support
ENV DOCKER_CONFIG=${HOME}/.docker

# Ruby suppport
ENV RUBYOPT="-KU -E utf-8:utf-8"

# Create the file system
COPY --from=base_os_context / /
COPY --from=binary_tools_context /out/ /
COPY --from=binary_tools_context /usr/local/go /usr/local/go
COPY --from=ruby_tools_context /usr/local/bin /usr/local/bin
COPY --from=ruby_tools_context /usr/local/lib /usr/local/lib
COPY --from=nodejs_tools_context /usr/local /usr/local
COPY --from=python_context /usr/local/bin /usr/local/bin
COPY --from=python_context /usr/local/lib /usr/local/lib

RUN mkdir -p ${HOME}/go && \
    mkdir -p ${HOME}/gobin && \
    mkdir -p ${HOME}/gocache && \
    mkdir -p ${DOCKER_CONFIG} && \
    mkdir -p ${HOME}/.config/gcloud && \
    mkdir -p ${HOME}/.kube && \
    mkdir -p ${HOME}/.helm

RUN chmod 777 ${HOME}/go && \
    chmod 777 ${HOME}/gobin && \
    chmod 777 ${HOME}/gocache && \
    chmod 777 ${DOCKER_CONFIG} && \
    chmod 777 ${HOME}/.config/gcloud && \
    chmod 777 ${HOME}/.kube && \
    chmod 777 ${HOME}/.helm

RUN git config --global url."git@github.ibm.com:".insteadOf "https://github.ibm.com/"
RUN ssh-keyscan github.ibm.com /root/.ssh/known_hosts

WORKDIR ${HOME}
