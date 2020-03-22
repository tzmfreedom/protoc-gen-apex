.PHONY: run
run: install
	protoc -I. \
	  -I${GOPATH}/src \
	  -I${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
		--apex_out="host=example.com/hoge":. hello.proto

.PHONY: instlal
install: format
	go install

.PHONY: format
format:
	gofmt -w .
