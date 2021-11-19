FROM ghcr.io/edwarnicke/govpp/vpp:20.09 as go
COPY --from=golang:1.16.3-buster /usr/local/go/ /go
ENV PATH ${PATH}:/go/bin
ENV GO111MODULE=on
ENV CGO_ENABLED=0
ENV GOBIN=/bin
RUN go get github.com/go-delve/delve/cmd/dlv@v1.6.0

FROM go as build
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . . 
RUN go build ./...

FROM build as test
CMD go test -test.v ./...

FROM test as debug
CMD dlv -l :40000 --headless=true --api-version=2 test -test.v .
