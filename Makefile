.PHONY: all test clean

default: all

all: fused

fused:
	GOOS=$(TARGET_OS) GOARCH=amd64 go build -o bin/fused fused

clean:
	rm -f bin/*

fmt:
	go fmt 

mod:
	go mod tidy -v

lint:
	golangci-lint run ./...

test: test_fused

test_fused: fused do_test_fused clean_test_fused

do_test_fused:
	test/test_fused

clean_test_fused:
	pkill -f bin/fused

test_fused_ubuntu:
	docker build -t test_fused_ubuntu --file=test/Dockerfile.test_fused_ubuntu .
	docker run -it -v /tmp/log:/tmp/log --privileged test_fused_ubuntu
