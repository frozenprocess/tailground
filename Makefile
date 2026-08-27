GO         ?= go
WASM_OUT   := web/public/engine.wasm
WASM_EXEC  := web/public/wasm_exec.js
TRIM_STUB  := engine/vendor-trim/config.go.stub
TRIM_DEST  := vendor/github.com/juanfont/headscale/hscontrol/types/config.go
CAP_PATCH  := engine/vendor-trim/capabilities.patch

.PHONY: vendor wasm wasm-exec spike clean

# go mod vendor + apply the vendor-trim (keeps the wasm small)
vendor:
	$(GO) mod vendor
	cp $(TRIM_STUB) $(TRIM_DEST)
	git apply --directory=vendor/github.com/juanfont/headscale $(CAP_PATCH)
	$(GO) build ./... >/dev/null && echo "vendor + trim OK"

wasm-exec:
	@mkdir -p web/public
	@install -m 0644 "$$($(GO) env GOROOT)/lib/wasm/wasm_exec.js" $(WASM_EXEC)

wasm: wasm-exec
	GOOS=js GOARCH=wasm $(GO) build -trimpath -ldflags "-s -w" -o $(WASM_OUT) ./engine/wasm
	@ls -lh $(WASM_OUT) | awk '{print "  " $$9 ": " $$5}'

# build the engine, then serve web/public on :8080
spike: wasm
	@echo "open http://localhost:8080/spike.html"
	@cd web/public && python3 -m http.server 8080

clean:
	rm -rf $(WASM_OUT) $(WASM_EXEC)
