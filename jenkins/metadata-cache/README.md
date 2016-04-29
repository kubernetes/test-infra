# Metadata server cache

Simple utility to cache requests sent to the metadata server.

The utility is composed of the following pieces:

* An http server which listens for metadata requests
  * If it is a token request, it caches the token
    - Automatically refreshes
    - Returns correct expiration time for each request
  * Otherwise it caches the response forever
* A script with commands to prepare the machine for the cache
  * Installs necessary packages
  * Starts/stops the cache
  * Updates /etc/hosts to control the resolution of  `metadata.google.internal`
    - Resolves to the internal ip when cache is on.
    - Resolves to the real metadata server on `169.254.169.254` when off.
* The script can also run commands without sshing to the remote machine:
  * Creates/deletes an instance
  * Copies files to the instance
  * Runs script commands on the instance
  * Grabs diagnostic information

## Instructions

### Quick setup

This is the ultimate lazy version, which does everything for you:

```sh
# Create a new instance
jenkins/metadata-cache/metadata-cache-control.sh remote_create $INSTANCE
# Update that instance
jenkins/metadata-cache/metadata-cache-control.sh remote_update $INSTANCE
```


### Detailed instructions

Please get help from the command for most up to date info:

```sh
# Command list
jenkins/metadata-cache/metadata-cache-control.sh help
# Remote command list
jenkins/metadata-cache/metadata-cache-control.sh remote_help
```

### Debugging info

```sh
# Run basic tests
jenkins/metadata-cache/metadata-cache-control.sh remote_ssh $INSTANCE test
# Print the configuration
jenkins/metadata-cache/metadata-cache-control.sh remote_ssh $INSTANCE cat
# Get logs
jenkins/metadata-cache/metadata-cache-control.sh remote_logs $INSTANCE
# Connect to the instance
jenkins/metadata-cache/metadata-cache-control.sh remote_ssh $INSTANCE connect
# Disable cache
jenkins/metadata-cache/metadata-cache-control.sh remote_ssh $INSTANCE off
```
