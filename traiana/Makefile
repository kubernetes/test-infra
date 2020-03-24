#tips
# might need to delete /usr/local/Cellar/protobuf if u get "google::protobuf" errors

#REGISTRY=520887410018.dkr.ecr.eu-west-1.amazonaws.com
#PROJECT=prow 

.PHONY: update-deps
update-deps:
	../hack/update-deps.sh

.PHONY: bazel-clean
bazel-clean:
	bazel clean

.PHONY: okro-release-push
okro-release-push:
	cd ../prow && \
	PROW_REPO_OVERRIDE=repo.okro.blue/platform-docker/prow EDGE_PROW_REPO_OVERRIDE=repo.okro.blue/platform-docker/prow bazel run //prow:okro-release-push --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64

.PHONY: ecr-login
ecr-login:
	export AWS_PROFILE=arch-$(USER); \
	$$(aws ecr get-login --no-include-email --region eu-west-1)
