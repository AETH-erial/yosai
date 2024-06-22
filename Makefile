.PHONY: build format test install coverage coverage-html

YOSAICTL = yosaictl

build:
	go build -x -v -o ./build/linux/$(YOSAICTL)/$(YOSAICTL) ./cmd/$(YOSAICTL)/$(YOSAICTL).go

format:
	go fmt ./...

install:
	sudo rm -f /usr/local/bin/$(YOSAICTL) && \
	sudo mv ./build/linux/$(YOSAICTL)/$(YOSAICTL) /usr/local/bin && sudo chmod u+x /usr/local/bin/$(YOSAICTL)


test:
	go test -v ./...

coverage:
	go test -v ./... -cover


coverage-html:
	go test -v ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
