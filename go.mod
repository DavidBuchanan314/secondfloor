module github.com/DavidBuchanan314/secondfloor

go 1.26.8

require (
	github.com/joho/godotenv v1.5.1
	github.com/syndtr/goleveldb v1.0.1-0.20220721030215-126854af5e6d
	golang.org/x/crypto v0.57.0
	google.golang.org/protobuf v1.36.12
)

require github.com/golang/snappy v0.0.4 // indirect

tool google.golang.org/protobuf/cmd/protoc-gen-go
