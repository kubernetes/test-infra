# start from standard clonerefs
ARG CLONEREFS_TAG=v20210225-6252cf69ff
ARG CLONEREFS_IMAGE=gcr.io/k8s-prow/clonerefs:${CLONEREFS_TAG}
FROM ${CLONEREFS_IMAGE}

# add prototype shim that handles pre-clones before running clonerefs
# TODO: move logic into clonerefs
RUN mv /clonerefs /clonerefs-real
COPY wrap-clonerefs /clonerefs
RUN chmod +x /clonerefs

# TODO: move clone_to_alias out into a helper tool
# TODO: select more repos to clone
RUN clone_to_alias(){ mkdir -p "/precloned/${2}" && git clone "${1}" "/precloned/${2}"; }; \
    clone_to_alias 'https://github.com/kubernetes/kubernetes' 'k8s.io/kubernetes'
