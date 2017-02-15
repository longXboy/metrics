FROM golang:1.7.5

ADD . $GOPATH/src/metrics

RUN go install metrics

CMD ["metrics"]