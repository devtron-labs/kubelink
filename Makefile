
all: build

TAG?=latest
FLAGS=
ENVVAR=
GOOS?=darwin
REGISTRY?=686244538589.dkr.ecr.us-east-2.amazonaws.com
BASEIMAGE?=alpine:3.9
GIT_COMMIT =$(shell sh -c 'git log --pretty=format:'%h' -n 1')
BUILD_TIME= $(shell sh -c 'date -u '+%Y-%m-%dT%H:%M:%SZ'')
GOFLAGS:= $(GOFLAGS) -buildvcs=false

include $(ENV_FILE)
export

build: clean
	$(ENVVAR) GOOS=$(GOOS) go build \
	    -o kubelink \

wire:
	wire

clean:
	rm -rf kubelink

run: build
	./kubelink

