FROM golang:1.16.10 AS build-env

RUN apt update
RUN apt install git gcc musl-dev make -y
RUN go get github.com/google/wire/cmd/wire
WORKDIR /go/src/github.com/devtron-labs/kubelink
ADD . /go/src/github.com/devtron-labs/kubelink/
RUN GOOS=linux make

FROM ubuntu
RUN apt update
RUN apt install ca-certificates -y
COPY --from=build-env  /go/src/github.com/devtron-labs/kubelink/kubelink .
CMD ["./kubelink"]
