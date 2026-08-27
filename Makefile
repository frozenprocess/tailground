GO         ?= go
WASM_OUT   := web/public/engine.wasm
WASM_EXEC  := web/public/wasm_exec.js
TRIM_STUB  := engine/vendor-trim/config.go.stub
TRIM_DEST  := vendor/github.com/juanfont/headscale/hscontrol/types/config.go
CAP_PATCH  := engine/vendor-trim/capabilities.patch

.PHONY: help vendor test build wasm wasm-exec size spike web-install web-dev web-build clean

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

clean:
	rm -rf bin $(WASM_OUT) $(WASM_EXEC) web/dist
