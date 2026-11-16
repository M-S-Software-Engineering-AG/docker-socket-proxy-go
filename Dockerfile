# syntax=docker/dockerfile:1

ARG BASE_IMAGE=gcr.io/distroless/static-debian13

FROM --platform=$BUILDPLATFORM golang:1.27-bookworm AS build
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0 GOOS=linux GOFLAGS=-trimpath
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} go build -ldflags="-s -w" -o /out/proxy .

FROM $BASE_IMAGE
COPY --from=build /out/proxy /proxy
USER 0:0
EXPOSE 2375
ENTRYPOINT ["/proxy"]
