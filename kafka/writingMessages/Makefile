
.PHONY: build
build:
	go build -o kfk .

.PHONY: compile
compile:
	protoc ./api/v1/proto/*.proto \
		--go_out=. \
		--go-grpc_out=. \
		--go_opt=paths=source_relative \
		--go-grpc_opt=paths=source_relative \
		--proto_path=.

