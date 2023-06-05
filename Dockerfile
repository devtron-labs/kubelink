FROM golang:1.18 AS build-env

RUN apt update
RUN apt install git gcc musl-dev make -y
RUN go install github.com/google/wire/cmd/wire@latest

WORKDIR /go/src/github.com/devtron-labs/kubelink
ADD . /go/src/github.com/devtron-labs/kubelink/
RUN GOOS=linux make

FROM ubuntu:22.04
RUN apt update
RUN apt install ca-certificates -y
RUN apt clean autoclean
RUN apt autoremove -y && rm -rf /var/lib/apt/lists/*
COPY --from=build-env  /go/src/github.com/devtron-labs/kubelink/kubelink .

RUN useradd -ms /bin/bash devtron
RUN chown -R devtron:devtron ./kubelink
USER devtron

CMD ["./kubelink"]
