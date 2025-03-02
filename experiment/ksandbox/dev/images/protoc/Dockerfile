# Copyright 2025 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Compile protoc ourselves, from source
FROM debian:latest AS protoc-builder

RUN apt-get update
RUN apt-get install -y g++ git cmake

WORKDIR /src
RUN git clone https://github.com/protocolbuffers/protobuf.git

WORKDIR /src/protobuf
RUN git checkout v3.12.4
RUN git submodule update --init --recursive

WORKDIR /src/protobuf/cmake
RUN cmake .
RUN cmake --build . -j8

RUN cp protoc /usr/local/bin

RUN /usr/local/bin/protoc --version

FROM golang:1.23.5

RUN apt-get update; apt-get install --yes unzip

RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28
RUN go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2

COPY --from=protoc-builder /usr/local/bin/protoc /usr/local/bin/protoc

ENTRYPOINT [ "/usr/local/bin/protoc" ]
