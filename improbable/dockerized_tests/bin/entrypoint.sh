#!/bin/bash

# Add local user
# Either use the LOCAL_USER_ID if passed in at runtime or
# fallback

USER_ID=${LOCAL_USER_ID:-9001}

useradd --shell /bin/bash -u "${USER_ID}" -o -c "" -m user
export HOME=/home/user
chown -R user:user /home/user

# Enable remote caching by default.
export BAZEL_REMOTE_CACHE_ENABLED=${BAZEL_REMOTE_CACHE_ENABLED:-true}
if [[ "${BAZEL_REMOTE_CACHE_ENABLED}" == "true" ]]; then
    echo "Bazel remote cache is enabled, generating .bazelrcs ..."
    /usr/local/bin/create_bazel_cache_rcs.sh
fi

exec /usr/local/bin/gosu user "$@"
