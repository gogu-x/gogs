include config.mk

SHELL := cmd.exe

proto:
	$(PYTHON) $(GEN_PROTO) $(PROTO_PATH) $(GO_OUT) $(MODULE)

register:
	$(PYTHON) $(GEN_REG) $(GO_OUT) $(PROTO_PATH)

build: proto register
	go build ./...
