FROM public.ecr.aws/docker/library/golang:1.22.2-alpine3.19 as builder

RUN apk update && \
    apk upgrade && \
    apk add busybox bash

RUN go install github.com/cespare/reflex@latest
ENV CGO_ENABLED=0

WORKDIR /go/src/app

CMD go mod tidy && \
    reflex -s -r '\.go$$' -- sh -c 'go run main.go'

