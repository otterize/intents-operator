# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION ?= 0.0.1

# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
# This variable is used to construct full image tags for bundle and catalog images.
#
# For example, running 'make bundle-build bundle-push catalog-build catalog-push' will build and push both
# otterize.com/operator-bundle:$VERSION and otterize.com/operator-catalog:$VERSION.
IMAGE_TAG_BASE ?= 353146681200.dkr.ecr.us-east-1.amazonaws.com/otterize

# Image URL to use all building/pushing image targets
IMG ?= $(IMAGE_TAG_BASE):operator-$(VERSION)

LOCAL_IMAGE_TAG ?= 255.255.255
OPERATOR_DEBUG_IMAGE ?= otterize/intents-operator:$(LOCAL_IMAGE_TAG)
WATCHER_DEBUG_IMAGE ?= otterize/intents-operator-pod-watcher:$(LOCAL_IMAGE_TAG)

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.24.1

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=otterize-intents-operator-manager-role crd webhook paths="../..."


.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="../..."
	go generate ../shared/...
	go generate ./...

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet envtest ## Run tests.
	go test ../exp/... -coverprofile cover.out
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./... -coverprofile cover.out

##@ Build

.PHONY: build
build: generate fmt vet ## Build manager binary.
	go build -o bin/manager main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./main.go

.PHONY: docker-build
docker-build: test ## Build docker image with the manager.
	docker build -t ${IMG} -f ../intents-operator.Dockerfile ../

.PHONY: docker-build-local
docker-build-local: test ## Build docker image with the manager.
	docker build -t ${OPERATOR_DEBUG_IMAGE} -f ../intents-operator.Dockerfile ../
	docker build -f ../watcher.Dockerfile -t ${WATCHER_DEBUG_IMAGE} ../


.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}

.PHONY: minikube-push
minikube-push: ## For MacOS users: Push locally built docker images directly into minikube VM since it doesn't access local docker registry directly.
	minikube ssh "docker rmi -f $(OPERATOR_DEBUG_IMAGE)"
	minikube image load ${OPERATOR_DEBUG_IMAGE}
	minikube ssh "docker rmi -f $(WATCHER_DEBUG_IMAGE)"
	minikube image load ${WATCHER_DEBUG_IMAGE}


##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

## Helm chart repo should be cloned as submodule
OTTERIZE_HELM_CHART_DIR ?= $(shell pwd)/../../helm-charts/otterize-kubernetes
$(OTTERIZE_HELM_CHART_DIR):
	git submodule update --init

.PHONY: helm-dependency
helm-dependency: $(OTTERIZE_HELM_CHART_DIR)
	cd $(OTTERIZE_HELM_CHART_DIR) && helm dependency build

.PHONY: copy-manifests-to-helm
copy-manifests-to-helm: manifests
	kubectl kustomize  ./config/webhook | sed -e "s/[']//g" > ./config/webhook/manifests-patched
	./copy-manifests-to-helm.sh

.PHONY: deploy
deploy: manifests copy-manifests-to-helm helm-dependency
	helm upgrade --install -n otterize-system --create-namespace otterize $(OTTERIZE_HELM_CHART_DIR)

.PHONY: deploy-local
deploy-local: manifests copy-manifests-to-helm helm-dependency ## Deploy images built locally into the cluster.
	helm upgrade --install -n otterize-system --create-namespace otterize $(OTTERIZE_HELM_CHART_DIR) \
	--set intentsOperator.operator.tag=$(LOCAL_IMAGE_TAG) \
	--set intentsOperator.operator.pullPolicy=Never \
	--set intentsOperator.watcher.tag=$(LOCAL_IMAGE_TAG) \
	--set intentsOperator.watcher.pullPolicy=Never

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	helm uninstall -n otterize-system otterize

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest

## Tool Versions
CONTROLLER_TOOLS_VERSION ?= v0.9.0

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
