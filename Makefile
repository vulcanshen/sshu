# sshu — build / run / test / package（家族共用的 target 在 family.mk）
#
# 跑 `make`（或 `make help`）列出所有指令。

BINARY := sshu
TAPES  := .local/demos/demo-grid.tape
include family.mk

##@ sshu 自己的

.PHONY: demo
demo: build ## 用 demo 的 hosts 跑（不碰你真正的 hosts.yaml）
	@mkdir -p $(DIST_DIR)/demo
	@cp -n demo/hosts.yaml $(DIST_DIR)/demo/hosts.yaml 2>/dev/null || true
	SSHU__CONFIG=$(DIST_DIR)/demo SSHU__DATA=$(DIST_DIR)/demo ./$(BINARY)
