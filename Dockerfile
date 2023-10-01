FROM golang:1.20.11 as go
ENV GO111MODULE=on
ENV CGO_ENABLED=0
ENV GOBIN=/bin
ARG BUILDARCH
RUN go install github.com/go-delve/delve/cmd/dlv@v1.8.2
ADD https://github.com/spiffe/spire/releases/download/v1.8.0/spire-1.8.0-linux-${BUILDARCH}-musl.tar.gz .
RUN tar xzvf spire-1.8.0-linux-${BUILDARCH}-musl.tar.gz -C /bin --strip=2 spire-1.8.0/bin/spire-server spire-1.8.0/bin/spire-agent

FROM go as build
WORKDIR /build
COPY go.mod go.sum ./
COPY ./internal/imports imports
RUN go build ./imports
COPY . .
RUN go build -o /bin/app .

FROM build as test
CMD go test -test.v ./...

FROM test as debug
CMD dlv -l :40000 --headless=true --api-version=2 test -test.v ./...

FROM alpine as runtime
COPY --from=build /bin/app /bin/app
ENTRYPOINT ["/bin/app"]