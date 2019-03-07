PROW_REPO_OVERRIDE=520887410018.dkr.ecr.eu-west-1.amazonaws.com/prow
#REGISTRY=520887410018.dkr.ecr.eu-west-1.amazonaws.com
#PROJECT=prow 

.PHONY: update-deps
update-deps:
	../hack/update-deps.sh

.PHONY: okro-release-push
okro-release-push:
	cd ../prow && \
	bazel run //prow:okro-release-push --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64