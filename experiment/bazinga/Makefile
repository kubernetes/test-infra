all: build

PROGRAM := bazinga
CGO_ENABLED ?= 0
LDFLAGS ?= -extldflags "-static" -w -s

build: $(PROGRAM)

.PHONY: $(PROGRAM)
$(PROGRAM):
	bazel build .
	CGO_ENABLED=$(CGO_ENABLED) go build -ldflags '$(LDFLAGS)' -o $@

.PHONY: test
test:
	bazel test .

.PHONY: clean
clean:
	bazel clean .
	go clean -i -x

.PHONY: example
example: CONFIG ?= examples/readme-config.yaml
example: $(PROGRAM)
	$< -config $(CONFIG)
