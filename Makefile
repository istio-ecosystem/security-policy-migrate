
build:
	go build -ldflags="-s -w" -o out/convert *.go

clean:
	rm -fr ./out/

.PHONY: build clean
