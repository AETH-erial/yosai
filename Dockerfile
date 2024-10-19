# syntax=docker/dockerfile:1

# Build the application from source
FROM golang:1.23 AS build

WORKDIR /

COPY . .
RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -o /yosai-server ./cmd/yosai-server/yosai-server.go

# Deploy the application binary into a lean image
FROM alpine:latest AS multi

WORKDIR /

COPY --from=build /yosai-server /yosai-server

RUN chmod ugo+x /yosai-server

EXPOSE 8080

CMD [ "/yosai-server" ] 
