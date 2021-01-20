
build:
	go build -ldflags="-s -w" -o out/convert *.go

test:
	go test -v ./...

clean:
	rm -fr ./out/

.PHONY: build clean
