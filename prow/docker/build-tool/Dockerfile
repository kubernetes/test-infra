#############
# Binary tools
#############

# Use golang as the binary_tools_context
FROM golang:1.15.7 as binary_tools_context

ARG ARCH=amd64

# Pinned versions of stuff
ENV CONTROLLER_GEN_VERSION=v0.4.0
ENV GOIMPORTS_VERSION=379209517ffe
ENV GOLANGCI_LINT_VERSION=v1.23.1
ENV HELM_VERSION=v3.4.1
ENV KUBECTL_VERSION=v1.16.0
ENV KUBEBUILDER_VERSION=2.3.1
ENV KUSTOMIZE_VERSION=v4.0.5
ENV OPERATOR_SDK_VERSION=v1.3.0
ENV KIND_VERSION=v0.10.0

ENV GO111MODULE=on
ENV GOPROXY=https://proxy.golang.org

WORKDIR /tmp
ENV GOPATH=/tmp/go

ENV OUTDIR=/out
RUN mkdir -p ${OUTDIR}/usr/bin
RUN mkdir -p ${OUTDIR}/usr/local
RUN mkdir -p ${OUTDIR}/usr/local/kubebuilder
RUN mkdir -p ${OUTDIR}/usr/lib

# hadolint ignore=DL3008
RUN apt-get update && apt-get install -y --no-install-recommends \
    apt-transport-https \
    build-essential \
    ca-certificates \
    curl \
    gnupg2 \
    software-properties-common

# Build and install Go tools
RUN go get -ldflags="-s -w" sigs.k8s.io/controller-tools/cmd/controller-gen@${CONTROLLER_GEN_VERSION}
RUN go get -ldflags="-s -w" golang.org/x/tools/cmd/goimports@${GOIMPORTS_VERSION}
RUN go get -ldflags="-s -w" github.com/golangci/golangci-lint/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}
RUN go get -ldflags="-s -w" sigs.k8s.io/kustomize/kustomize/v4@${KUSTOMIZE_VERSION}
RUN go get -ldflags="-s -w" sigs.k8s.io/kind@${KIND_VERSION}

# operator-sdk
RUN OPERATOR_SDK_ARCH=$(echo ${ARCH}| sed 's/amd64/x86_64/g'); \
    curl -L https://github.com/operator-framework/operator-sdk/releases/download/${OPERATOR_SDK_VERSION}/operator-sdk-${OPERATOR_SDK_VERSION}-${OPERATOR_SDK_ARCH}-linux-gnu -o ${OUTDIR}/usr/bin/operator-sdk; \
    chmod 555 ${OUTDIR}/usr/bin/operator-sdk

# kubebuilder installation only for x86_64 and ppc64le platforms
RUN set -eux; \
    if [ ${ARCH} != "s390x" ] ; then \
        curl -L https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KUBEBUILDER_VERSION}/kubebuilder_${KUBEBUILDER_VERSION}_linux_${ARCH}.tar.gz -o /tmp/kubebuilder_${KUBEBUILDER_VERSION}_linux_${ARCH}.tar.gz ; \
        tar -xf /tmp/kubebuilder_${KUBEBUILDER_VERSION}_linux_${ARCH}.tar.gz  -C /tmp; \
        mv /tmp/kubebuilder_${KUBEBUILDER_VERSION}_linux_${ARCH}/bin ${OUTDIR}/usr/local/kubebuilder; \
    fi;

# Helm
ADD https://get.helm.sh/helm-${HELM_VERSION}-linux-${ARCH}.tar.gz /tmp
RUN tar -xf /tmp/helm-${HELM_VERSION}-linux-${ARCH}.tar.gz -C /tmp
RUN mv /tmp/linux-${ARCH}/helm ${OUTDIR}/usr/bin

# Kubectl
ADD https://storage.googleapis.com/kubernetes-release/release/${KUBECTL_VERSION}/bin/linux/${ARCH}/kubectl ${OUTDIR}/usr/bin/kubectl
RUN chmod 555 ${OUTDIR}/usr/bin/kubectl

# Move go tools to OUTDIR
RUN mv /tmp/go/bin/* ${OUTDIR}/usr/bin

# Cleanup stuff we don't need in the final image
RUN rm -fr /usr/local/go/doc
RUN rm -fr /usr/local/go/test
RUN rm -fr /usr/local/go/api
RUN rm -fr /usr/local/go/bin/godoc
RUN rm -fr /usr/local/go/bin/gofmt


#############
# Base OS
#############

FROM ubuntu:bionic as base_os_context

ENV DOCKER_VERSION=18.06.3~ce~3-0~ubuntu

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

# Install docker
ADD https://download.docker.com/linux/ubuntu/gpg /tmp/docker-key
RUN apt-key add /tmp/docker-key
RUN add-apt-repository "deb [arch=$(dpkg --print-architecture)] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable"
RUN apt-get update && apt-get -y install --no-install-recommends \
    docker-ce="${DOCKER_VERSION}" \
 && apt-get clean \
 && rm -rf /var/lib/apt/lists/*

# Clean up unused stuff for the final image
RUN rm -rf /var/lib/apt/lists/*
RUN rm -fr /usr/share/bash-completion
RUN rm -fr /usr/share/bug
RUN rm -fr /usr/share/doc
RUN rm -fr /usr/share/locale
RUN rm -fr /usr/share/man
RUN rm -fr /tmp/*

# Run dockerd in CI
COPY docker-entrypoint.sh /usr/local/bin/entrypoint
RUN chmod +x /usr/local/bin/entrypoint


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
ENV PATH=/usr/lib/google-cloud-sdk/bin:/usr/local/go/bin:/usr/local/kubebuilder/bin:${HOME}/go/bin:${HOME}/gobin:$PATH

# Docker support
ENV DOCKER_CONFIG=${HOME}/.docker

# Create the file system
COPY --from=base_os_context / /
COPY --from=binary_tools_context /out/ /
COPY --from=binary_tools_context /usr/local/go /usr/local/go

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

# Docker in Docker settings
VOLUME /var/lib/docker
EXPOSE 2375

WORKDIR ${HOME}

ENTRYPOINT ["/usr/local/bin/entrypoint"]
