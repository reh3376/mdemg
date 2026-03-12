.PHONY: build clean test

MODULE_NAME = uxts-module

build:
	go build -o $(MODULE_NAME) .

clean:
	rm -f $(MODULE_NAME)

test:
	go test -v ./...
