variable "REGISTRY" {
  default = "ghcr.io/m-s-software-engineering-ag"
}

variable "VERSION" {
  default = "v1.0.0"
}

variable "GIT_SHA" {
  default = "dev"
}

# release.yml sets this to "true" only for the highest tag, so a patch on
# an older release line never moves :latest backwards.
variable "LATEST" {
  default = "false"
}

group "default" {
  targets = ["docker-socket-proxy", "docker-socket-proxy-debug"]
}

target "docker-socket-proxy" {
  context    = "."
  dockerfile = "Dockerfile"
  tags       = concat(
    [
      "${REGISTRY}/docker-socket-proxy-go:${VERSION}",
      "${REGISTRY}/docker-socket-proxy-go:${GIT_SHA}",
    ],
    LATEST == "true" ? ["${REGISTRY}/docker-socket-proxy-go:latest"] : [],
  )
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
  tags     = concat(
    [
      "${REGISTRY}/docker-socket-proxy-go:${VERSION}-debug",
      "${REGISTRY}/docker-socket-proxy-go:${GIT_SHA}-debug",
    ],
    LATEST == "true" ? ["${REGISTRY}/docker-socket-proxy-go:latest-debug"] : [],
  )
}
