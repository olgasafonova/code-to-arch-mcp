.PHONY: build test check lint vet fmt-check clean

BINARY := code-to-arch
CMD := ./cmd/code-to-arch

build:
	go build -o $(BINARY) $(CMD)

test:
	go test -race -failfast ./...

test-cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run ./...

vet:
	go vet ./...

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Files need formatting:" && gofmt -l . && exit 1)

check: fmt-check vet test

clean:
	rm -f $(BINARY) coverage.out coverage.html

# Cross-platform builds
dist:
	GOOS=darwin GOARCH=arm64 go build -o dist/$(BINARY)-darwin-arm64 $(CMD)
	GOOS=darwin GOARCH=amd64 go build -o dist/$(BINARY)-darwin-amd64 $(CMD)
	GOOS=linux GOARCH=amd64 go build -o dist/$(BINARY)-linux-amd64 $(CMD)
	GOOS=linux GOARCH=arm64 go build -o dist/$(BINARY)-linux-arm64 $(CMD)
	GOOS=windows GOARCH=amd64 go build -o dist/$(BINARY)-windows-amd64.exe $(CMD)
