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

.PHONY: help vendor test build wasm wasm-exec size spike web-install web-dev web-build e2e e2e-offline e2e-engine e2e-down image image-push clean

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

clean:
	rm -rf bin $(WASM_OUT) $(WASM_EXEC) web/dist
