.PHONY: run
run: install
	protoc --apex_out=. hello.proto

.PHONY: instlal
install: format
	go install

.PHONY: format
format:
	gofmt -w .
