
build:
	go build -o out/convert *.go

clean:
	rm -fr ./out/

.PHONY: build clean
