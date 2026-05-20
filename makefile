include config.mk

proto:
	protoc --proto_path=$(PROTO_DIR) \
		--go_out=$(ROOT_DIR) --go_opt=module=$(MODULE) \
		--go-grpc_out=$(ROOT_DIR) --go-grpc_opt=module=$(MODULE) \
		$(shell find $(PROTO_DIR) -name "*.proto")
