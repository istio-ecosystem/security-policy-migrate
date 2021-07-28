
build:
	@go build -ldflags="-s -w -X main.version=$(shell ./get-version.sh)" -o out/convert *.go

test:
	@go list ./... | grep -v /e2e | xargs go test -v

e2e:
	go test -v ./e2e/...

release: build test
	@cd out && tar -czvf convert.tar.gz convert

clean:
	@rm -fr ./out/

.PHONY: build test clean release e2e
