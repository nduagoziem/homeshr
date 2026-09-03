-include .env

.PHONY: up status down create

GOOSE ?= goose
GOOSE_DIR ?= internal/migrations
DB_URL ?= $(DATABASE_URL)
MIGRATION_NAME := $(word 2,$(MAKECMDGOALS))

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
