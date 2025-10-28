.PHONY: build clean run

BINARY_NAME=echo-agent

build:
	go build -o $(BINARY_NAME) .

clean:
	rm -f $(BINARY_NAME)

run: build
	./$(BINARY_NAME)

install-deps:
	go mod download
	go mod tidy
