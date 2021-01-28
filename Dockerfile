FROM golang:1.15.5

RUN apt-get update && apt-get install -y --force-yes netcat jq && apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

WORKDIR /go/src/app
COPY ./main.go ./main.go
COPY ./bee-staging.yml ./bee-staging.yml

RUN go get -d -v ./...
RUN go install -v ./...

RUN wget -q -O - https://raw.githubusercontent.com/ethersphere/bee/master/install.sh | TAG=v0.4.2 bash
RUN mkdir /go/src/app/data

COPY ./run-bee.sh ./run-bee.sh

CMD ["bash", "/go/src/app/run-bee.sh"]
