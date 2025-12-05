ARTIFACTS_DIR := $(PWD)/hack/_artifacts
BUILD_DEST := $(PWD)/_build
CHARTS_DIR := $(PWD)/charts
#CHARTS_LIST := $(patsubst $(CHARTS_DIR)/%,%,$(wildcard $(CHARTS_DIR)/*))
CHARTS_LIST := $(notdir $(wildcard $(CHARTS_DIR)/*))

CHARTS_VERSION ?= $(shell ${PWD}/get_version.sh)
REGISTRY ?= registry.ci.mirantis.com
OCI_REPOSITORY ?= $(REGISTRY)/k0rdent-bm/charts

define BUILD_CHARTS

.PHONY: build-charts
build-charts: ${1}-build-chart

.PHONY: ${1}-build-chart
${1}-build-chart:
	helm dependency update $(CHARTS_DIR)/$(1)
	helm package $(CHARTS_DIR)/$(1)  --destination $(BUILD_DEST) --version $(CHARTS_VERSION)
endef

define LINT_CHARTS

.PHONY: lint-charts
lint-charts: ${1}-lint-chart

.PHONY: ${1}-lint-chart
${1}-lint-chart:
	helm lint --strict $(CHARTS_DIR)/$(1)
endef

define PUSH_CHARTS

.PHONY: push-charts
push-charts: ${1}-push-chart

.PHONY: ${1}-push-chart
${1}-push-chart:
	helm push $(BUILD_DEST)/$(1)-$(CHARTS_VERSION).tgz oci://$(OCI_REPOSITORY)
endef

$(foreach chart,$(CHARTS_LIST),\
  $(eval $(call BUILD_CHARTS,$(chart)))\
  $(eval $(call LINT_CHARTS,$(chart)))\
  $(eval $(call PUSH_CHARTS,$(chart)))\
)

.PHONY: build
build: build-charts

.PHONY: lint
lint: lint-charts

.PHONY: push
push: push-charts

.PHONY: version
version:
	@echo ${CHARTS_VERSION}
