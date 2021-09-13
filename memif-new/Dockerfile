FROM ghcr.io/edwarnicke/govpp/vpp:20.09 as go
COPY --from=golang:1.15.3-buster /usr/local/go/ /go
ENV PATH ${PATH}:/go/bin
ENV GO111MODULE=on
ENV CGO_ENABLED=0
ENV GOBIN=/bin
ENV GOPROXY=https://goproxy.io,direct
RUN rm -r /etc/vpp
RUN go get github.com/go-delve/delve/cmd/dlv@latest

FROM go as build
WORKDIR /workdir

FROM ghcr.io/edwarnicke/govpp/vpp:20.09 as runtime

