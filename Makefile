
build:
	@go build -ldflags="-s -w -X main.version=$(shell ./get-version.sh)" -o out/convert *.go

test:
	@go test -v $(go list ./... | grep -v /e2e)

e2e:
	@go test -v $(go list ./... | grep /e2e)

release: build test
	@cd out && tar -czvf convert.tar.gz convert

clean:
	@rm -fr ./out/

.PHONY: build test clean release
