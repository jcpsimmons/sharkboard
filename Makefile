# sharkboard: build and check targets.

BIN := sharkboard

.PHONY: build test vet fmt run once clean

build:
	go build -o $(BIN) .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

run: build
	./$(BIN)

once: build
	./$(BIN) -once -wait 10s

clean:
	rm -f $(BIN) sharksend
