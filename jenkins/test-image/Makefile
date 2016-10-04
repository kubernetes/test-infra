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

IMG = gcr.io/k8s-testimages/kubekins-test
TAG = $(shell date +v%Y%m%d)-$(shell git describe --tags --always --dirty)
FILES := $(wildcard ./Dockerfile-*)

do_build = \
	$(eval FILETAG := $(subst ./Dockerfile-,,$(f)))\
	docker build -f $(f) -t $(IMG):$(FILETAG)-$(TAG) . ;\
	docker tag -f $(IMG):$(FILETAG)-$(TAG) $(IMG):$(FILETAG)-latest ;\
	echo Built $(IMG):$(FILETAG)-$(TAG) and tagged with $(IMG):$(FILETAG)-latest ;

do_push = \
	$(eval FILETAG := $(subst ./Dockerfile-,,$(f)))\
	gcloud docker push $(IMG):$(FILETAG)-$(TAG);\
	gcloud docker push $(IMG):$(FILETAG)-latest;\
	echo Pushed $(IMG):$(FILETAG)-latest and $(IMG):$(FILETAG)-$(TAG);

all: build

build:
	@$(foreach f,$(FILES),$(do_build))

push: build
	@$(foreach f,$(FILES),$(do_push))

