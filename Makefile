include Makefile.api
include Makefile.kind

# commerce-auth's and commerce-obs's options ship no Go code: generate it
# from the commits buf.lock pins before the service's own code, which
# imports it. A prerequisite, so Makefile.api's generate stays dedo's.
.PHONY: generate-options
generate-options:
	buf generate --template buf.gen.options.yaml

generate: generate-options

# The integration tests (app/) create a database of their own in this
# Postgres; make kind-up exposes the cluster's at 127.0.0.1:15432. Without
# it they skip.
export TEST_DATABASE_URL ?= postgres://postgres:postgres@127.0.0.1:15432/postgres?sslmode=disable

# ---------- the whole stack in kind: service, BFF, frontend ----------
#
#   make kind-all     cluster + build all three images + load + deploy
#   make kind-smoke   health, docs and MCP answer
#
# The BFF and the frontend are paired by the commit they are built from:
# both get BUILD_SHA, the first 12 characters of HEAD (bfftools.BuildSHACommand).
BUILD_SHA := $(shell git rev-parse HEAD 2>/dev/null | cut -c1-12)
KIND_BFF_IMAGE      ?= localhost/dedo-todo-bff
KIND_FRONTEND_IMAGE ?= localhost/dedo-todo-frontend
KIND_FRONTEND_PORT  := $(shell awk '/^[[:space:]]*-/ { c = ""; h = "" } /containerPort:/ { c = $$NF } /hostPort:/ { h = $$NF } c == 30081 && h != "" { print h; exit }' zarf/k8s/dev/kind-config.yaml 2>/dev/null)

.PHONY: kind-vendor kind-all kind-all-update kind-bff-build kind-frontend-build kind-all-load kind-all-apply

## kind-all: create the kind cluster if needed, then build, load and deploy the service, its BFF and its frontend
kind-all: kind-cluster
	$(MAKE) kind-all-update KIND_TAG=$(KIND_TAG)
	@echo
	@echo "App   http://localhost:$(KIND_FRONTEND_PORT)   (sign in as alice, bob or reader)"
	@echo "Docs  http://localhost:$(KIND_PORT)/docs"
	@echo "MCP   http://localhost:$(KIND_PORT)/mcp  (headers: Authorization: Bearer alice, location-id: loc-demo)"

## kind-all-update: rebuild all three images at one tag, load them and roll the deployments
kind-all-update: kind-vendor
	$(MAKE) -j1 kind-build kind-bff-build kind-frontend-build kind-all-load kind-all-apply KIND_TAG=$(KIND_TAG)

# The images build offline from modules vendored here, on the host, which
# can reach the private ones (GOPRIVATE). .build/ is not committed.
kind-vendor:
	rm -rf $(KIND_BUILD)/vendor && mkdir -p $(KIND_BUILD)/vendor
	go mod vendor -o $(KIND_BUILD)/vendor/service
	cd bff && go mod vendor -o ../$(KIND_BUILD)/vendor/bff

kind-bff-build: $(KIND_NO_AUTH)
	$(CONTAINER) build $(KIND_AUTH_FLAG) -f zarf/docker/dockerfile.bff \
		--build-arg BUILD_SHA=$(BUILD_SHA) -t $(KIND_BFF_IMAGE):$(KIND_TAG) .

kind-frontend-build: $(KIND_NO_AUTH)
	$(CONTAINER) build $(KIND_AUTH_FLAG) -f zarf/docker/dockerfile.frontend \
		--build-arg BUILD_SHA=$(BUILD_SHA) -t $(KIND_FRONTEND_IMAGE):$(KIND_TAG) .

kind-all-load:
	mkdir -p $(KIND_BUILD)
	for image in $(KIND_IMAGE) $(KIND_BFF_IMAGE) $(KIND_FRONTEND_IMAGE); do \
		$(CONTAINER) save -o $(KIND_BUILD)/image.tar $$image:$(KIND_TAG) && \
		kind load image-archive $(KIND_BUILD)/image.tar --name $(KIND_CLUSTER) && \
		rm -f $(KIND_BUILD)/image.tar || exit 1; \
	done

# The overlay pins tag "dev"; this build's tag is swapped in at render
# time, so a deploy never changes a tracked file.
kind-all-apply:
	mkdir -p $(KIND_BUILD)
	$(KIND_KUBECTL) kustomize zarf/k8s/dev \
		| sed -e 's|$(KIND_IMAGE):dev|$(KIND_IMAGE):$(KIND_TAG)|' \
		      -e 's|$(KIND_BFF_IMAGE):dev|$(KIND_BFF_IMAGE):$(KIND_TAG)|' \
		      -e 's|$(KIND_FRONTEND_IMAGE):dev|$(KIND_FRONTEND_IMAGE):$(KIND_TAG)|' > $(KIND_BUILD)/dev.yaml
	$(KIND_KUBECTL) apply -f $(KIND_BUILD)/dev.yaml
	$(KIND_KUBECTL) -n $(KIND_NAMESPACE) rollout status deployment/postgres --timeout=180s
	$(KIND_KUBECTL) -n $(KIND_NAMESPACE) rollout status deployment/dedo-todo --timeout=180s
	$(KIND_KUBECTL) -n $(KIND_NAMESPACE) rollout status deployment/bff --timeout=120s
	$(KIND_KUBECTL) -n $(KIND_NAMESPACE) rollout status deployment/frontend --timeout=120s

## kind-claude-entry: print Claude Desktop's mcpServers entry for the cluster's /mcp, as
## KIND_CLAUDE_TOKEN in loc-demo: dedo's entry plus the location-id header a user's token needs
KIND_CLAUDE_LOCATION ?= loc-demo
.PHONY: kind-claude-entry
kind-claude-entry:
	@go tool dedo local-mcp-claude -token $(KIND_CLAUDE_TOKEN) -print | python3 -c 'import json,sys; e=json.load(sys.stdin); [v["args"].extend(["--header","location-id: $(KIND_CLAUDE_LOCATION)","--header","Version: v4"]) for v in e.values()]; print(json.dumps(e, indent=2))'
