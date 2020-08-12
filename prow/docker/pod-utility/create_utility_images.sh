#!/bin/bash
# © Copyright IBM Corporation 2020.
# LICENSE: Apache License, Version 2.0 (http://www.apache.org/licenses/LICENSE-2.0)
#
# Instructions:
# Execute build script: bash create_utility_images.sh    
#

CURDIR="$(pwd)"
LOG_FILE="${CURDIR}/logs/csi-pod-utilities-$(date +"%F-%T").log"

#Check if directory exists
if [ ! -d "$CURDIR/logs/" ]; then
   mkdir -p "$CURDIR/logs/"
fi

function checkDocker() {
	printf -- "\nChecking if Docker is already present on the system . . . \n" 
	if [ -x "$(command -v docker)" ]; then
		docker --version | grep "Docker version" 
		echo "Docker exists !!" 
		docker ps 2>&1 
	else
		printf -- "\n Please install and run Docker first !! \n" 
		exit 1
	fi
}

checkDocker | tee -a "$LOG_FILE"
docker build -t pod-utilities -f  Dockerfile.podutilitybinaries . | tee -a "$LOG_FILE"
docker build -t clonerefs-s390x -f Dockerfile.clonerefs . | tee -a "$LOG_FILE"
docker build -t initupload-s390x -f Dockerfile.initupload . | tee -a "$LOG_FILE"
docker build -t entrypoint-s390x -f Dockerfile.entrypoint . | tee -a "$LOG_FILE"
docker build -t sidecar-s390x -f Dockerfile.sidecar . | tee -a "$LOG_FILE"
docker rmi pod-utilities | tee -a "$LOG_FILE"
