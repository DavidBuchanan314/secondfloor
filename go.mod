module github.com/DavidBuchanan314/secondfloor

go 1.26.8

require (
	github.com/fsnotify/fsnotify v1.10.1
	github.com/joho/godotenv v1.5.1
	github.com/syndtr/goleveldb v1.0.1-0.20220721030215-126854af5e6d
	go.senan.xyz/taglib v0.14.0
	golang.org/x/crypto v0.57.0
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/golang/snappy v0.0.4 // indirect
	github.com/tetratelabs/wazero v1.11.1-0.20260428013916-2bbd517b7633 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

tool google.golang.org/protobuf/cmd/protoc-gen-go
