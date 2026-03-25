.PHONY: build clean test run

MODULE_NAME = echo-module

build:
	go build -o $(MODULE_NAME) .

clean:
	rm -f $(MODULE_NAME)

test:
	go test -v ./...

run: build
	./$(MODULE_NAME) --socket /tmp/$(MODULE_NAME).sock
