FROM golang:1.20.6-bullseye

RUN sed -i s/deb.debian.org/mirrors.tuna.tsinghua.edu.cn/g /etc/apt/sources.list

RUN apt update && apt install -y git-buildpackage nano

# Install deepin dch plugin scripts
RUN curl -fLo /usr/lib/python3/dist-packages/gbp/scripts/deepin_changelog.py https://raw.githubusercontent.com/deepin-community/deepin-gbp-dch-plugins/master/scripts/deepin_changelog.py && chmod +x /usr/lib/python3/dist-packages/gbp/scripts/deepin_changelog.py

# Use `docker build --platform=linux/arm64 --platform=linux/amd64 -t hub.deepin.com/prow/hook-base .` make hook base image
