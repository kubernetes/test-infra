#!/bin/bash

# Manifest file path
MANIFEST_PATH="/path/dummy"

# Date range for checking images
DATE_RANGE="start to end date dummy"

# Run kpromo command to check unsigned images in dry run mode
kpromo sigcheck --date-range "$DATE_RANGE" --dry-run --manifest $MANIFEST_PATH