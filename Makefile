# Figure out which Zarf binary we should use based on the operating system we are on
ZARF_BIN := ./build/zarf
UNAME_S := $(shell uname -s)
UNAME_P := $(shell uname -p)
# Provide a default value for the operating system architecture used in tests, e.g. " APPLIANCE_MODE=true|false make test-e2e ARCH=arm64"
ARCH ?= amd64
ifneq ($(UNAME_S),Linux)
	ifeq ($(UNAME_S),Darwin)
		ZARF_BIN := $(addsuffix -mac,$(ZARF_BIN))
	endif
	ifeq ($(UNAME_P),i386)
		ZARF_BIN := $(addsuffix -intel,$(ZARF_BIN))
	endif
	ifeq ($(UNAME_P),arm)
		ZARF_BIN := $(addsuffix -apple,$(ZARF_BIN))
	endif
endif

CLI_VERSION := $(if $(shell git describe --tags), $(shell git describe --tags), "UnknownVersion")
BUILD_ARGS := -s -w -X 'github.com/defenseunicorns/zarf/src/config.CLIVersion=$(CLI_VERSION)'
.DEFAULT_GOAL := help

.PHONY: help
help: ## Show a list of all targets
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
	| sed -n 's/^\(.*\): \(.*\)##\(.*\)/\1:\3/p' \
	| column -t -s ":"

remove-packages: ## remove all zarf packages recursively
	find . -type f -name 'zarf-package-*' -delete

vm-init: ## usage -> make vm-init OS=ubuntu
	vagrant destroy -f
	vagrant up --no-color ${OS}
	echo -e "\n\n\n\033[1;93m  ✅ BUILD COMPLETE.  To access this environment, run \"vagrant ssh ${OS}\"\n\n\n"

vm-destroy: ## Destroy the VM
	vagrant destroy -f

clean: ## Clean the build dir
	rm -rf build

destroy:
	$(ZARF_BIN) destroy --confirm --remove-components
	rm -fr build

build-cli-linux-amd: build-injector-registry
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(BUILD_ARGS)" -o build/zarf main.go

build-cli-linux-arm: build-injector-registry
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="$(BUILD_ARGS)" -o build/zarf-arm main.go

build-cli-mac-intel: build-injector-registry
	GOOS=darwin GOARCH=amd64 go build -ldflags="$(BUILD_ARGS)" -o build/zarf-mac-intel main.go

build-cli-mac-apple: build-injector-registry
	GOOS=darwin GOARCH=arm64 go build -ldflags="$(BUILD_ARGS)" -o build/zarf-mac-apple main.go

build-cli-linux: build-cli-linux-amd build-cli-linux-arm

build-cli: build-cli-linux-amd build-cli-linux-arm build-cli-mac-intel build-cli-mac-apple ## Build the CLI

build-injector-registry:
	cd src/injector/stage2 && $(MAKE) build-bootstrap-registry

# Inject and deploy a new dev version of zarf agent for testing (should have an existing zarf agent deployemt)
# @todo: find a clean way to support Kind or k3d: k3d image import $(tag)
dev-agent-image:
	$(eval tag := defenseunicorns/dev-zarf-agent:$(shell date +%s))
	$(eval arch := $(shell uname -m))
	CGO_ENABLED=0 GOOS=linux go build -o build/zarf-linux-$(arch) main.go
	DOCKER_BUILDKIT=1 docker build --tag $(tag) --build-arg TARGETARCH=$(arch) . && \
	kind load docker-image $(tag) && \
	kubectl -n zarf set image deployment/agent-hook server=$(tag)

init-package: ## Create the zarf init package, macos "brew install coreutils" first
	@test -s $(ZARF_BIN) || $(MAKE) build-cli

	$(ZARF_BIN) package create -o build -a $(ARCH) --confirm .

ci-release: init-package ## Create the init package

build-examples:
	@test -s $(ZARF_BIN) || $(MAKE) build-cli

	@test -s ./build/zarf-package-dos-games-$(ARCH).tar.zst || $(ZARF_BIN) package create examples/game -o build -a $(ARCH) --confirm

	@test -s ./build/zarf-package-component-scripts-$(ARCH).tar.zst || $(ZARF_BIN) package create examples/component-scripts -o build -a $(ARCH) --confirm

	@test -s ./build/zarf-package-component-choice-$(ARCH).tar.zst || $(ZARF_BIN) package create examples/component-choice -o build -a $(ARCH) --confirm

	@test -s ./build/zarf-package-component-variables-$(ARCH).tar.zst || $(ZARF_BIN) package create examples/component-variables -o build -a $(ARCH) --confirm

	@test -s ./build/zarf-package-data-injection-demo-$(ARCH).tar || $(ZARF_BIN) package create examples/data-injection -o build -a $(ARCH) --confirm

	@test -s ./build/zarf-package-gitops-service-data-$(ARCH).tar.zst || $(ZARF_BIN) package create examples/gitops-data -o build -a $(ARCH) --confirm

	@test -s ./build/zarf-package-test-helm-releasename-$(ARCH).tar.zst || $(ZARF_BIN) package create examples/helm-alt-release-name -o build -a $(ARCH) --confirm

	@test -s ./build/zarf-package-compose-example-$(ARCH).tar.zst || $(ZARF_BIN) package create examples/composable-packages -o build -a $(ARCH) --confirm

	@test -s ./build/zarf-package-flux-test-${ARCH}.tar.zst || $(ZARF_BIN) package create examples/flux-test -o build -a $(ARCH) --confirm

## Run e2e tests. Will automatically build any required dependencies that aren't present. 
## Requires an existing cluster for the env var APPLIANCE_MODE=true
.PHONY: test-e2e
test-e2e: init-package build-examples 
	cd src/test/e2e && go test -failfast -v -timeout 30m
