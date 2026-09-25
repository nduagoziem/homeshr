-include .env

.PHONY: up status down create proto vendor-proto run

GOOSE ?= goose
GOOSE_DIR ?= internal/migrations
DB_URL ?= $(DATABASE_URL)
MIGRATION_NAME := $(word 2,$(MAKECMDGOALS))

# Proto / gRPC codegen
MODULE      := github.com/nduagoziem/homeshr/services/listing
GOBIN       := $(shell go env GOPATH)/bin
ANNOTATIONS := proto/third_party/google/api/annotations.proto
PROTO_FILES := $(wildcard proto/*.proto)

ifneq ($(MIGRATION_NAME),)
$(eval $(MIGRATION_NAME):; @:)
endif

# Goose Commands
create: # E.g: make create create_bookings_table
	@if [ -z "$(MIGRATION_NAME)" ]; then echo "Usage: make create <migration_name>"; exit 1; fi
	$(GOOSE) -dir $(GOOSE_DIR) create $(MIGRATION_NAME) sql

up: # E.g: make up
	$(GOOSE) -dir $(GOOSE_DIR) postgres "$(DB_URL)" up

status: # E.g: make status
	$(GOOSE) -dir $(GOOSE_DIR) postgres "$(DB_URL)" status

down: # E.g make down
	$(GOOSE) -dir $(GOOSE_DIR) postgres "$(DB_URL)" down

# Vendored import protos (googleapis annotations + descriptor.proto). Fetched
# once; protoc needs them on the include path to resolve google/api/*.
$(ANNOTATIONS):
	@mkdir -p proto/third_party/google/api proto/third_party/google/protobuf
	curl -fsSL -o proto/third_party/google/api/annotations.proto \
		https://raw.githubusercontent.com/googleapis/googleapis/master/google/api/annotations.proto
	curl -fsSL -o proto/third_party/google/api/http.proto \
		https://raw.githubusercontent.com/googleapis/googleapis/master/google/api/http.proto
	curl -fsSL -o proto/third_party/google/protobuf/descriptor.proto \
		https://raw.githubusercontent.com/protocolbuffers/protobuf/v3.19.6/src/google/protobuf/descriptor.proto

vendor-proto: $(ANNOTATIONS) # fetch the vendored import protos

# Generate Go bindings (proto/*.pb.go and proto/*_grpc.pb.go). The central
# Envoy descriptor is generated from services/Makefile.
proto: $(ANNOTATIONS)
	@for proto_file in $(PROTO_FILES); do \
		PATH="$(PATH):$(GOBIN)" protoc \
			-I proto -I proto/third_party \
			--go_out=. --go_opt=module=$(MODULE) \
			--go-grpc_out=. --go-grpc_opt=module=$(MODULE) \
			"$$proto_file"; \
	done

run: # start the gRPC booking server on :50052 (Envoy's upstream)
	go run .
