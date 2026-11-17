# syntax=docker/dockerfile:1

ARG BASE_IMAGE=gcr.io/distroless/static-debian13

FROM --platform=$BUILDPLATFORM golang:1.27-bookworm AS build
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

# go.mod sets the minimum Go version; auto fetches it if the image is older.
ENV GOTOOLCHAIN=auto
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
