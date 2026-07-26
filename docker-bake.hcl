variable "REGISTRY" {
  default = "ghcr.io/basecrusher"
}

variable "VERSION" {
  default = "v1.0.0"
}

variable "GIT_SHA" {
  default = "dev"
}

group "default" {
  targets = ["docker-socket-proxy", "docker-socket-proxy-debug"]
}

target "docker-socket-proxy" {
  context    = "."
  dockerfile = "Dockerfile"
  tags       = [
    "${REGISTRY}/docker-socket-proxy-go:${VERSION}",
    "${REGISTRY}/docker-socket-proxy-go:${GIT_SHA}",
    "${REGISTRY}/docker-socket-proxy-go:latest",
  ]
  platforms  = [
    "linux/amd64",
    "linux/arm64",
    "linux/arm/v7",
  ]
}

target "docker-socket-proxy-debug" {
  inherits = ["docker-socket-proxy"]
  args     = {
    BASE_IMAGE = "gcr.io/distroless/static-debian13:debug"
  }
  tags     = [
    "${REGISTRY}/docker-socket-proxy-go:${VERSION}-debug",
    "${REGISTRY}/docker-socket-proxy-go:${GIT_SHA}-debug",
    "${REGISTRY}/docker-socket-proxy-go:latest-debug",
  ]
}
