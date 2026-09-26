include config.mk

SHELL := cmd.exe

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

.PHONY: deepcopy-tool deepcopy

DEEPCOPY_GEN_BIN := tools\deepcopy-gen.exe

deepcopy-tool:
	go build -o $(DEEPCOPY_GEN_BIN) ./tools/deepcopy-gen

deepcopy: deepcopy-tool
	$(DEEPCOPY_GEN_BIN)

build: c proto register deepcopy
