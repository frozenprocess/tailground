GO         ?= go
WASM_OUT   := web/public/engine.wasm
WASM_EXEC  := web/public/wasm_exec.js
TRIM_STUB  := engine/vendor-trim/config.go.stub
TRIM_DEST  := vendor/github.com/juanfont/headscale/hscontrol/types/config.go
CAP_PATCH  := engine/vendor-trim/capabilities.patch

IMAGE_REPO := ghcr.io/frozenprocess/tailground
# Every build is pushed twice: as :latest (what the cluster runs) and as a
# unique <timestamp>-<sha> tag (the archive, for rollbacks). `:=` freezes the
# timestamp so both tags in one make invocation match.
TAG        ?= $(shell date +%Y%m%d-%H%M%S)-$(shell git rev-parse --short HEAD)
TAG        := $(TAG)
IMAGE      := $(IMAGE_REPO):$(TAG)

.PHONY: help test build wasm wasm-exec size spike vendor clean web-install web-dev web-build e2e e2e-engine e2e-offline e2e-down image image-push require-host k8s-gateway k8s-deploy k8s-rollback k8s-backup

help:
	@echo "Tailground:"
	@echo "  make vendor      - go mod vendor + apply the vendor-trim (run after any dep bump)"
	@echo "  make test        - go test ./..."
	@echo "  make build       - build the CLI harness (bin/tg-eval)"
	@echo "  make wasm        - build web/public/engine.wasm (+ wasm_exec.js)"
	@echo "  make size        - build wasm and report raw + gzip sizes"
	@echo "  make spike       - make wasm, then serve web/public on :8080 (open /spike.html)"
	@echo "  make web-install - npm install for the SPA"
	@echo "  make web-dev     - build the engine, then run the Vite dev server"
	@echo "  make web-build   - build the engine + the static SPA into web/dist"
	@echo "  make e2e         - differential harness: every case vs a real disposable tailnet (needs creds + Docker)"
	@echo "  make e2e-offline - PR-safe offline harness allowlist (never selects live tests)"
	@echo "  make e2e-engine  - offline: score every case against its authored expect (no creds/Docker)"
	@echo "  make e2e-down    - delete every child tailnet the harness owns"
	@echo "  make image       - build the deploy container image ($(IMAGE))"
	@echo "  make image-push  - build + push the image"
	@echo "  make k8s-gateway - add the tailground listener to the cluster gateway (once)"
	@echo "  make k8s-deploy  - kubectl apply -k deploy/k8s, restart onto :latest, wait for rollout"
	@echo "  make k8s-backup - download a consistent snapshot of the short-link DB into backups/"
	@echo "  make k8s-rollback TAG=... - re-point :latest at an archived build, then k8s-deploy"
	@echo "  (ship a fresh build: make image-push k8s-deploy)"

vendor:
	$(GO) mod vendor
	cp $(TRIM_STUB) $(TRIM_DEST)
	git apply --directory=vendor/github.com/juanfont/headscale $(CAP_PATCH)
	$(GO) build ./... >/dev/null && echo "vendor + trim OK"

test:
	$(GO) test ./...

build:
	$(GO) build -o bin/tg-eval ./engine/cli

wasm: wasm-exec
	GOOS=js GOARCH=wasm $(GO) build -trimpath -ldflags "-s -w" -o $(WASM_OUT) ./engine/wasm
	@ls -lh $(WASM_OUT) | awk '{print "  " $$9 ": " $$5}'

wasm-exec:
	@mkdir -p web/public
	@install -m 0644 "$$($(GO) env GOROOT)/lib/wasm/wasm_exec.js" $(WASM_EXEC)

size: wasm
	@echo "raw:  $$(wc -c < $(WASM_OUT) | numfmt --to=iec)"
	@echo "gzip: $$(gzip -9 -c $(WASM_OUT) | wc -c | numfmt --to=iec)"
	@command -v brotli >/dev/null && echo "brotli: $$(brotli -q11 -c $(WASM_OUT) | wc -c | numfmt --to=iec)" || echo "brotli: (not installed; ~= gzip x 0.85)"

spike: wasm
	@echo "open http://localhost:8080/spike.html"
	@cd web/public && python3 -m http.server 8080

web-install:
	cd web && npm install

web-dev: wasm
	cd web && npm run dev

web-build: wasm
	cd web && npm run build
	@echo "static site in web/dist"

# `go test` runs each package's binary with its CWD set to the package dir
# (e2e/), so a relative E2E_ARTIFACTS / E2E_REPORT would resolve against e2e/,
# not the repo root the examples above assume. Anchor them to absolute paths.
e2e_paths = $(if $(E2E_ARTIFACTS),E2E_ARTIFACTS='$(abspath $(E2E_ARTIFACTS))') $(if $(E2E_REPORT),E2E_REPORT='$(abspath $(E2E_REPORT))')

e2e:
	@set -a; [ -f .env ] && . ./.env; [ -f e2e/.run/creds.env ] && . e2e/.run/creds.env; set +a; \
	 $(e2e_paths) $(GO) test -tags e2e -count=1 -v -timeout 60m -run 'TestE2E$$|TestEngineCases|TestE2EFuzz|TestFuzzDeterministic|TestShrinkCandidates' ./e2e/...

# PR-safe allowlist: never selects live tests, even with credentials in the environment.
e2e-offline:
	$(GO) test -tags e2e -count=1 -run '^(TestEngineCases|TestFuzzPoliciesValid|TestFuzzDeterministic|TestShrinkCandidates|TestAwaitConverged|TestClassifyRows)$$' ./e2e/...

e2e-engine:
	$(e2e_paths) $(GO) test -tags e2e -count=1 -v -run 'TestEngineCases|TestE2EFuzz|TestFuzzDeterministic|TestShrinkCandidates' ./e2e/...

## e2e-down — delete every child tailnet the harness owns (via its persisted
## creds in e2e/.run/) and report any tg-e2e- tailnet it can't reach. Set
## E2E_TAILNET_MAX_AGE to spare recent ones.
e2e-down:
	@set -a; [ -f .env ] && . ./.env; [ -f e2e/.run/creds.env ] && . e2e/.run/creds.env; set +a; \
	 E2E_REAP=1 $(GO) test -tags e2e -count=1 -v -run TestReap ./e2e/...

## deploy — container image + Kubernetes (see deploy/README.md)
image:
	docker build -f deploy/Dockerfile -t $(IMAGE) -t $(IMAGE_REPO):latest .

image-push: image
	docker push $(IMAGE)
	docker push $(IMAGE_REPO):latest

# Deploy settings, substituted into deploy/k8s/*.yaml via envsubst. Override on
# the command line or from the environment, e.g.
#   make k8s-deploy TAILGROUND_HOST=tailground.example.com
TAILGROUND_HOST   ?=
IMAGE_TAG         ?= latest
GATEWAY_NAME      ?= cluster-gateway
GATEWAY_NAMESPACE ?= default
STORAGE_SIZE      ?= 1Gi
# Empty = the cluster's default StorageClass.
STORAGE_CLASS     ?=

K8S_VARS := TAILGROUND_HOST IMAGE_REPO IMAGE_TAG GATEWAY_NAME GATEWAY_NAMESPACE STORAGE_SIZE STORAGE_CLASS
export $(K8S_VARS)
K8S_ENVSUBST := envsubst '$(foreach v,$(K8S_VARS),$$$(v))'

require-host:
	@test -n "$(TAILGROUND_HOST)" || { echo "set TAILGROUND_HOST=<hostname>"; exit 1; }

k8s-gateway: require-host
	$(K8S_ENVSUBST) < deploy/k8s/gateway-listener-patch.yaml > $(or $(TMPDIR),/tmp)/tg-gw-patch.yaml
	kubectl patch gateway $(GATEWAY_NAME) -n $(GATEWAY_NAMESPACE) \
	  --type=json --patch-file $(or $(TMPDIR),/tmp)/tg-gw-patch.yaml

# With IMAGE_TAG=latest (imagePullPolicy: Always) the spec never changes between
# builds, so force a rollout to make the pods re-pull.
k8s-deploy: require-host
	kubectl kustomize deploy/k8s | $(K8S_ENVSUBST) | kubectl apply -f -
	kubectl -n tailground rollout restart deploy/tailground
	kubectl -n tailground rollout status deploy/tailground

# Re-point :latest at an archived build (registry-side, no pull), then roll.
k8s-rollback:
	@test -n "$(filter command line,$(origin TAG))" || { echo "usage: make k8s-rollback TAG=<timestamp>-<sha>"; exit 1; }
	docker buildx imagetools create -t $(IMAGE_REPO):latest $(IMAGE)
	$(MAKE) k8s-deploy

# Download the production short-link database. The API takes a consistent
# snapshot itself (VACUUM INTO — safe while serving, unlike copying the WAL-mode
# file), it is streamed down, and the remote copy is removed. Needs an image
# that has the -backup flag (any build from 04311b6 onward).
BACKUP_DIR  ?= backups
BACKUP_FILE := $(BACKUP_DIR)/shares-$(shell date -u +%Y%m%dT%H%M%SZ).db
# kubectl honours $KUBECONFIG from the environment (or `make k8s-backup KUBECONFIG=...`).
k8s-backup:
	@mkdir -p $(BACKUP_DIR)
	kubectl -n tailground exec deploy/tailground -c api -- \
	  sh -c 'rm -f /data/backup.tmp.db && /usr/local/bin/tailground-api -db /data/shares.db -backup /data/backup.tmp.db'
	kubectl -n tailground exec deploy/tailground -c api -- cat /data/backup.tmp.db > $(BACKUP_FILE).part
	kubectl -n tailground exec deploy/tailground -c api -- rm -f /data/backup.tmp.db
	@mv $(BACKUP_FILE).part $(BACKUP_FILE)
	@echo "wrote $(BACKUP_FILE) ($$(wc -c < $(BACKUP_FILE)) bytes)"

clean:
	rm -rf bin $(WASM_OUT) $(WASM_EXEC) web/dist
