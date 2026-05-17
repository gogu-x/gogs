include config.mk

proto:
	protoc --proto_path=$(PROTO_DIR) \
		--go_out=$(ROOT_DIR) --go_opt=module=$(MODULE) \
		--go-grpc_out=$(ROOT_DIR) --go-grpc_opt=module=$(MODULE) \
		$(wildcard $(PROTO_DIR)/*.proto)
	lua $(ROOT_DIR)/tools/gen-register.lua \
		$(PROTO_DIR)/game.proto \
		$(ROOT_DIR)/pb/game \
		game
