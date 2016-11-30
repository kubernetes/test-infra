#!/bin/bash

if [ "$#" -ne 1 ]; then
  echo "usage: $0 [program]"
  exit 1
fi

cd $(dirname $0)

makefile_version_re="^\(${1^^}_VERSION.*=\s*\)"
version=$(sed -n "s/$makefile_version_re//p" Makefile)
new_version=$(awk -F. '{print $1 "." $2+1}' <<< $version)

echo "program: $1"
echo "old version: $version"
echo "new version: $new_version"

sed -i "s/$makefile_version_re/\1$new_version/" Makefile
sed -i "s/\(${1,,}:\)[0-9.]*/\1$new_version/" cluster/*.yaml
