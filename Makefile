.PHONY: build format test install coverage coverage-html

YOSAICTL = yosaictl
YOSAID = yosaid
YOSAISERVER = yosai-server

build:
	go build -o ./build/linux/$(YOSAICTL)/$(YOSAICTL) ./cmd/$(YOSAICTL)/$(YOSAICTL).go && \
		go build -o ./build/linux/$(YOSAID)/$(YOSAID) ./cmd/$(YOSAID)/$(YOSAID).go && \
		go build -o ./build/linux/${YOSAISERVER}/$(YOSAISERVER) ./cmd/$(YOSAISERVER)/$(YOSAISERVER).go
format:
	go fmt ./...

install:
	sudo mv ./build/linux/$(YOSAICTL)/$(YOSAICTL) /usr/local/bin && sudo chmod u+x /usr/local/bin/$(YOSAICTL) && \
	sudo mv ./build/linux/$(YOSAID)/$(YOSAID) /usr/local/bin && sudo chmod u+x /usr/local/bin/$(YOSAID)


test:
	go test -v ./...

coverage:
	go test -v ./... -cover


coverage-html:
	go test -v ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
