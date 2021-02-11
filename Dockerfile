FROM golang:1.15.5

RUN apt-get update && apt-get install -y --force-yes netcat jq git-core && apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

WORKDIR /go/src/app
COPY ./main.go ./main.go
COPY ./bee-staging.yml ./bee-staging.yml

RUN go get -d -v ./...
RUN go install -v ./...

RUN mkdir -p /go/src/github.com/ethersphere
RUN git clone --depth=1 --branch v0.5.0 https://github.com/ethersphere/bee.git /go/src/github.com/ethersphere/bee
RUN cd /go/src/github.com/ethersphere/bee && make binary

COPY ./run-bee.sh /go/src/app/run-bee.sh

CMD ["/bin/bash", "/go/src/app/run-bee.sh"]
