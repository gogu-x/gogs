PROTO_PATH := /data/g1/protocol
GO_OUT     := .
MODULE     := github.com/gogu-x/gogs
PYTHON     := python3
GEN_PROTO  := tools/gen-proto.py
GEN_REG    := tools/gen-register.py
GEN_CONF   := tools/gen-conf.py
GEN_CONF_DATA := tools/import-conf.py
CONF_PATH  := /data/g1/glconf
CONF_OUT   := ./glconf
CONF_MONGO_URI=mongodb://127.0.0.1:27017/?authSource=admin
CONF_MONGO_DB=gs_conf_g1_dev
