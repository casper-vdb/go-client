GO ?= go
CASPER_GRPC_PROTO_PATH := proto
CASPER_GRPC_SOURCES := $(shell find $(CASPER_GRPC_PROTO_PATH) -type f -name "*.proto")

.PHONY: generate-casper-grpc
generate-casper-grpc: $(CASPER_GRPC_SOURCES)
	@echo "Generating casper GRPC proto..."
	$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.2
	$(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.3.0
	mkdir -p ./pkg/proto
	protoc --proto_path=./proto --go_out=./pkg/proto --go-grpc_out=./pkg/proto --go_opt=paths=source_relative --go-grpc_opt=paths=source_relative $(CASPER_GRPC_SOURCES)


