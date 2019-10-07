# Based of the Dockerfile from https://github.com/gohugoio

FROM golang:1.11-stretch AS build


WORKDIR /go/src/github.com/lectio/imap-facade-openproject
RUN apt-get install \
    git gcc g++ binutils
COPY . /go/src/github.com/lectio/imap-facade-openproject/
ENV GO111MODULE=on
RUN go get -d .

ARG CGO=0
ENV CGO_ENABLED=${CGO}
ENV GOOS=linux

# default non-existent build tag so -tags always has an arg
ARG BUILD_TAGS="99notag"
RUN go install -ldflags '-w -extldflags "-static"' -tags ${BUILD_TAGS}

# ---

FROM alpine:3.9
RUN apk add --no-cache ca-certificates
COPY --from=build /go/bin/imap-facade-openproject /imap-facade
ARG  WORKDIR="/app"
WORKDIR ${WORKDIR}
VOLUME  ./conf ${WORKDIR}/conf
VOLUME  ./certs ${WORKDIR}/certs
EXPOSE  443/tcp
EXPOSE  2143/tcp
ENTRYPOINT [ "/imap-facade" ]
CMD [ "run" ]
