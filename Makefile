PROTOS := $(shell find proto -name '*.proto')

.PHONY: proto
proto:
	go build -o .bin/protoc-gen-go google.golang.org/protobuf/cmd/protoc-gen-go
	protoc --plugin=protoc-gen-go=.bin/protoc-gen-go -I proto --go_out=. --go_opt=module=github.com/DavidBuchanan314/secondfloor $(PROTOS)
