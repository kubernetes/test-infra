# Copyright 2016 The Kubernetes Authors.
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

VERSION ?= dev

loader: loader.go
	CGO_ENABLED=0 go build

.PHONY: docker
docker: loader
ifeq ("", "$(wildcard apparmor_parser)")
	$(error Missing required apparmor_parser binary)
endif
ifeq ("", "$(wildcard libapparmor.so.1)")
	$(error Missing required libapparmor.so.1 library)
endif
	docker build -t google/apparmor-loader:$(VERSION) -f Dockerfile .

.PHONY: clean
clean:
	rm -f loader
