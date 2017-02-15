FROM golang:1.7.5

EXPOSE 6060

ADD . $GOPATH/src/metrics

RUN go install metrics

CMD ["metrics"]