include config.mk

#SHELL := cmd.exe

proto:
	$(PYTHON) $(GEN_PROTO) $(PROTO_PATH) $(GO_OUT) $(MODULE)

register:
	$(PYTHON) $(GEN_REG) $(GO_OUT) $(PROTO_PATH)

.PHONY: conf-code conf-data c

conf-code:
	$(PYTHON) $(GEN_CONF) "$(CONF_OUT)" "$(CONF_PATH)"

conf-data:
	$(PYTHON) $(GEN_CONF_DATA) "$(CONF_MONGO_URI)" "$(CONF_MONGO_DB)" "$(CONF_PATH)"

c: conf-code conf-data

.PHONY: deepcopy accessor router

# 生成目录相对于仓库根目录，支持命令行覆盖。
GEN_DIR ?= .

deepcopy:
	"$(DEEPCOPY_GEN_BIN)" -dir "$(GEN_DIR)"

accessor:
	"$(ACCESSOR_GEN_BIN)" -dir "$(GEN_DIR)" $(STRUCTS)

router:
	"$(ROUTER_GEN_BIN)" -dir "$(GEN_DIR)"

build: c proto register deepcopy

.PHONY: install-generators
# 将本仓库生成器安装到已在 PATH 中的用户目录。
install-generators:
	bash tools/install-generators.sh
