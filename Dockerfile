
FROM --platform=$BUILDPLATFORM golang:1.24 as builder

ARG TARGETPLATFORM
ARG ENVVAR=CGO_ENABLED=0

WORKDIR /go/src/bitbucket.org/atlassian-developers/mini-proxy

COPY go.mod go.sum Makefile ./
COPY cmd cmd
COPY config config
COPY internal internal

RUN make build-linux ENVVAR=$ENVVAR

FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /go/src/bitbucket.org/atlassian-developers/mini-proxy/bin/linux/mini-proxy /bin/mini-proxy

CMD [ "/bin/mini-proxy" ]
