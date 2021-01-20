
build:
	go build -ldflags="-s -w" -o out/convert *.go

test:
	go test -v ./...

release: build
	cd out && tar -czvf convert.tar.gz convert

clean:
	rm -fr ./out/

.PHONY: build clean
