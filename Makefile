PROTOS := $(shell find proto -name '*.proto')

.PHONY: proto
proto:
	protoc --plugin=protoc-gen-go=$(shell go tool -n protoc-gen-go) -I proto --go_out=. --go_opt=module=github.com/DavidBuchanan314/secondfloor $(PROTOS)
