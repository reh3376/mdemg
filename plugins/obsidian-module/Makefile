MODULE := obsidian-module

.PHONY: build clean test

build:
	go build -o $(MODULE) .

clean:
	rm -f $(MODULE)

test:
	go test -v ./...
