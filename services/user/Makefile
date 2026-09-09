-include .env

.PHONY: up status down create proto vendor-proto run

GOOSE ?= goose
GOOSE_DIR ?= internal/migrations
DB_URL ?= $(DATABASE_URL)
MIGRATION_NAME := $(word 2,$(MAKECMDGOALS))

# Proto / gRPC codegen
MODULE      := github.com/nduagoziem/homeshr/services/user
GOBIN       := $(shell go env GOPATH)/bin
ANNOTATIONS := proto/third_party/google/api/annotations.proto

ifneq ($(MIGRATION_NAME),)
$(eval $(MIGRATION_NAME):; @:)
endif

# Goose Commands
create: # E.g: make create create_refresh_token_table
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

# Generate Go bindings (proto/*.pb.go) and the binary descriptor set
# (proto/auth.pb) that Envoy's grpc_json_transcoder loads.
proto: $(ANNOTATIONS)
	PATH="$(PATH):$(GOBIN)" protoc \
		-I proto -I proto/third_party \
		--go_out=. --go_opt=module=$(MODULE) \
		--go-grpc_out=. --go-grpc_opt=module=$(MODULE) \
		--descriptor_set_out=proto/auth.pb --include_imports --include_source_info \
		proto/auth.proto

run: # start the gRPC auth server on :50051 (Envoy's upstream)
	go run .

# Envoy is not run from here: it is a Portainer-managed container
# (homeshr-user-service-envoy, image envoyproxy/envoy:v1.31-latest) created with:
#   - command:      envoy -c /etc/envoy/envoy.yaml
#   - port:         32768 -> 10000
#   - bind mounts:  ./envoy.yaml      -> /etc/envoy/envoy.yaml (ro)
#                   ./proto/auth.pb   -> /etc/envoy/auth.pb    (ro)
#   - extra host:   host.docker.internal:host-gateway   (so it can reach `make run` on :50051)
# After editing envoy.yaml or regenerating auth.pb, restart that container in Portainer.
