.PHONY: envoy-proto

PROTOC ?= protoc

envoy-proto:
	$(PROTOC) \
		-I user/proto \
		-I booking/proto \
		-I user/proto/third_party \
		--descriptor_set_out=envoy.pb \
		--include_imports \
		--include_source_info \
		user/proto/user.proto \
		booking/proto/booking.proto
