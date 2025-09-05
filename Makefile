default: install

build:
@go generate
@go build

install:
@go generate
@go install

.PHONY: build install
