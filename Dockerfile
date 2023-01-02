FROM golang:latest as builder
ARG CGO_ENABLED=0

COPY ./ /root/src/
WORKDIR /root/src/
RUN go build -ldflags "-s -w" -trimpath -o pinger

FROM alpine:latest
COPY --from=builder /root/src/pinger /usr/bin/
