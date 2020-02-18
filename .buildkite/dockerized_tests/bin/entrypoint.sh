#!/bin/bash

# Add local user
# Either use the LOCAL_USER_ID if passed in at runtime or
# fallback

USER_ID=${LOCAL_USER_ID:-9001}

useradd --shell /bin/bash -u "${USER_ID}" -o -c "" -m user
export HOME=/home/user
chown -R user:user /home/user

exec /usr/local/bin/gosu user "$@"
