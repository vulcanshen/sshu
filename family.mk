# family.mk — terminu family 共用的 make target（tdp project/family.mk）
#
# 全家族同一份，不在 app 裡改：要改先改 tdp，再複製到每個 app。
# app 的 Makefile 先設變數、再 include 它，自己的 target 寫在 include 後面：
#
#   BINARY := kamonu
#   include family.mk
#
#   ##@ kamonu 自己的
#   …
#
# 變數（include 之前設）：
#   BINARY      必填：執行檔的名字，也是 cmd/ 底下那個目錄的名字
#   CHECK       接在 check 後面跑的 app 自己的 target（例：mod-check）
#   GOFMT_DIRS  gofmt 管的目錄，預設 cmd internal
#   TAPES       make gif 跑的 VHS tape，預設 .local/demos/demo.tape
#   VHS         vhs 的路徑，預設 vhs

ifndef BINARY
$(error 先設 BINARY 再 include family.mk)
endif

# 這個檔自己的路徑：include 進來時它是 MAKEFILE_LIST 的最後一個。
FAMILY_MK := $(lastword $(MAKEFILE_LIST))

PKG        := ./cmd/$(BINARY)
DIST_DIR   := dist
GOFMT_DIRS ?= cmd internal
TAPES      ?= .local/demos/demo.tape
# vhs 0.12.0 有時兩秒就結束、不出檔也不報錯（webu 踩過）；0.12.1 正常。
# 要指定別的版本：make gif VHS=/path/to/vhs
VHS        ?= vhs

GOOS    := $(shell go env GOOS)
GOARCH  := $(shell go env GOARCH)
# 只用在打包的檔名；執行檔本身的版本是 dev，goreleaser 發版時才注入。
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w

.DEFAULT_GOAL := help

##@ 編譯（build）

.PHONY: build
build: ## 編譯本地執行檔 → ./$(BINARY)（CGO_ENABLED=0 靜態、-trimpath、strip；版本是 dev）
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)
	@echo "built ./$(BINARY)  ($(VERSION) $(GOOS)/$(GOARCH))"

.PHONY: install
install: ## go install → $GOBIN，否則 $GOPATH/bin
	CGO_ENABLED=0 go install -trimpath -ldflags "$(LDFLAGS)" $(PKG)

.PHONY: uninstall
uninstall: ## 移除 make install 裝的 $(BINARY)
	@dir="$$(go env GOBIN)"; [ -n "$$dir" ] || dir="$$(go env GOPATH)/bin"; \
	if [ -f "$$dir/$(BINARY)" ]; then rm -f "$$dir/$(BINARY)" && echo "removed $$dir/$(BINARY)"; else echo "not installed ($$dir/$(BINARY))"; fi

.PHONY: tidy
tidy: ## go mod tidy（關掉 go.work，照 go.mod 寫的版本）
	GOWORK=off go mod tidy

##@ 發布（release）

.PHONY: release-check
# goreleaser 把 brews 標成 deprecated，check 因此回失敗；家族刻意用 formula（理由在
# .goreleaser.yaml），所以只有這一種失敗不算，警告照樣印出來。
release-check: ## goreleaser check：驗證 .goreleaser.yaml（brews 的 deprecated 警告照印、不算失敗）
	@out=$$(goreleaser check 2>&1); rc=$$?; echo "$$out"; \
	[ $$rc -eq 0 ] || echo "$$out" | grep -q "configuration is valid, but uses deprecated properties"

.PHONY: snapshot
snapshot: ## goreleaser 本機打包（不發布、不推 tap）→ dist/；關掉 go.work，跟 CI 一樣
	GOWORK=off goreleaser release --snapshot --clean

##@ 執行（run）

.PHONY: run
run: ## 本地跑（讀真正的設定與資料）
	go run $(PKG)

.PHONY: dev
dev: build ## 編譯再跑 ./$(BINARY)（跟 build 同一個執行檔；讀真正的設定與資料）
	./$(BINARY)

##@ 測試 / 檢查（test）

.PHONY: test
test: ## 跑所有測試（有 go.work 時用本機的 lib）
	go test ./...

.PHONY: test-race
test-race: ## 帶 race detector 跑測試（CI 跑的就是這個）
	go test -race ./...

.PHONY: check-lib
check-lib: ## 關掉 go.work，照 go.mod 寫的版本編譯、測試（過不了：用到了 lib 還沒發版的東西）
	GOWORK=off go build ./...
	GOWORK=off go test ./...

.PHONY: vet
vet: ## go vet ./...
	go vet ./...

.PHONY: fmt
fmt: ## gofmt -w（就地格式化）
	gofmt -w $(GOFMT_DIRS)

.PHONY: fmt-check
fmt-check: ## gofmt -l（列出沒格式化的檔；有輸出就失敗）
	@out=$$(gofmt -l $(GOFMT_DIRS)); if [ -n "$$out" ]; then echo "not gofmt'ed:"; echo "$$out"; exit 1; fi

.PHONY: check
check: fmt-check vet test-race $(if $(wildcard go.work),check-lib) $(CHECK) ## commit 前一次跑：fmt-check、vet、test-race；有 go.work 時加 check-lib；最後是 app 的 CHECK

##@ 打包（package）

.PHONY: package
package: build ## 打包本機平台 → dist/$(BINARY)_<版本>_<os>_<arch>.tar.gz
	@mkdir -p $(DIST_DIR)
	tar -czf $(DIST_DIR)/$(BINARY)_$(VERSION)_$(GOOS)_$(GOARCH).tar.gz $(BINARY)
	@echo "packaged $(DIST_DIR)/$(BINARY)_$(VERSION)_$(GOOS)_$(GOARCH).tar.gz"

##@ 展示（demo）

.PHONY: gif
gif: build ## 用 VHS 錄 README 的 demo gif（需 vhs、JetBrainsMono Nerd Font；tape 在 .local/demos/）
	@for t in $(TAPES); do echo "$(VHS) $$t"; $(VHS) "$$t" || exit 1; done

##@ 其他

.PHONY: clean
clean: ## 移除 ./$(BINARY) 與 dist/
	rm -f $(BINARY)
	rm -rf $(DIST_DIR)

.PHONY: help
help: ## 顯示這份說明（家族共用的先列，app 自己的在後面）
	@awk -v bin="$(BINARY)" 'BEGIN {FS = ":.*## "} \
		/^##@/ {printf "\n\033[1m%s\033[0m\n", substr($$0, 5); next} \
		/^[a-zA-Z0-9_-]+:.*## / {d = $$2; gsub(/\$$\(BINARY\)/, bin, d); printf "  \033[36m%-14s\033[0m %s\n", $$1, d}' \
		$(FAMILY_MK) $(filter-out $(FAMILY_MK),$(MAKEFILE_LIST))
