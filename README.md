# docker-socket-proxy-go

Tiny Go proxy in front of `/var/run/docker.sock`. It exposes the Docker
Engine REST API on a TCP port with per-section allowlists and a
default-on read-only mode. Inspired by `linuxserver/docker-socket-proxy`,
but distroless: a single static binary, no shell, no package manager.

Unlike the other images in this family it runs as **root** (uid/gid `0`).
It exists to be the one container that touches `/var/run/docker.sock`,
which is `root:docker` on the host — an unprivileged uid cannot read it,
and the alternatives (`group_add` the socket's gid, which differs per
host) buy nothing here. Everything in front of it stays rootless and
talks to this proxy over TCP.

Reference spec: [Docker Engine API v1.54](https://docs.docker.com/reference/api/engine/version/v1.54/).
Env-var names mirror `linuxserver/docker-socket-proxy` /
`Tecnativa/docker-socket-proxy` so existing compose snippets port over.
**All API gates default to `false`** (deny). `READONLY` defaults to
`true`.

## Operational env vars

| Env            | Default                | Meaning                                                                               |
| -------------- | ---------------------- | ------------------------------------------------------------------------------------- |
| `LISTEN_ADDR`  | `0.0.0.0:2375`         | TCP bind                                                                              |
| `DOCKER_SOCKET`| `/var/run/docker.sock` | Upstream unix socket                                                                  |
| `LOG_LEVEL`    | `info`                 | `debug` \| `info` \| `warn` \| `error`                                                |
| `LOG_FORMAT`   | `console`              | `console` (slog text, key=value) \| `json`. Anything but `json` is console.           |
| `LOG_PAYLOAD`  | `false`                | When `true`, audit log includes req + resp body capped at 8 KiB. Streams skipped.     |
| `READONLY`     | `true`                 | When `true`, only `GET`/`HEAD` permitted on enabled APIs. Set `false` to allow writes.|

## API gate env vars

Set to `true` to allow that section. Anything else (unset, `false`,
`0`, garbage) is deny.

| Env            | Path(s)                        |
| -------------- | ------------------------------ |
| `AUTH`         | `/auth`                        |
| `BUILD`        | `/build`, `/build/prune`       |
| `COMMIT`       | `/commit`                      |
| `CONFIGS`      | `/configs/*`                   |
| `CONTAINERS`   | `/containers/*`                |
| `DISTRIBUTION` | `/distribution/*`              |
| `EVENTS`       | `/events`                      |
| `EXEC`         | `/exec/*`                      |
| `GRPC`         | `/grpc` (BuildKit, future)     |
| `IMAGES`       | `/images/*`                    |
| `INFO`         | `/info`                        |
| `NETWORKS`     | `/networks/*`                  |
| `NODES`        | `/nodes/*`                     |
| `PING`         | `/_ping`                       |
| `PLUGINS`      | `/plugins/*`                   |
| `SECRETS`      | `/secrets/*`                   |
| `SERVICES`     | `/services/*`                  |
| `SESSION`      | `/session`                     |
| `SWARM`        | `/swarm/*`                     |
| `SYSTEM`       | `/system/*` (e.g. `/system/df`)|
| `TASKS`        | `/tasks/*`                     |
| `VERSION`      | `/version`                     |
| `VOLUMES`      | `/volumes/*`                   |


## Audit log

Every request emits one structured `slog` record. Default (`LOG_FORMAT=console`):

```
time=... level=INFO msg=audit remote=172.18.0.5:54321 method=GET path=/v1.54/containers/json section=CONTAINERS decision=allow status=200 duration_ms=4
```

With `LOG_FORMAT=json`:

```json
{
  "time": "...",
  "level": "INFO",
  "msg": "audit",
  "remote": "172.18.0.5:54321",
  "method": "GET",
  "path": "/v1.54/containers/json",
  "section": "CONTAINERS",
  "decision": "allow",
  "status": 200,
  "duration_ms": 4
}
```

`decision` is one of: `allow`, `deny:section_disabled`, `deny:readonly`,
`deny:unknown_path`. With `LOG_PAYLOAD=true` records also include
`req_body` and `resp_body` (capped 8 KiB each).

## Run

```sh
docker run --rm -d --name dsp \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -e CONTAINERS=true -e PING=true -e VERSION=true \
  -p 127.0.0.1:2375:2375 \
  ghcr.io/m-s-software-engineering-ag/docker-socket-proxy-go:latest
```

Then:

```sh
docker -H tcp://127.0.0.1:2375 ps          # 200
docker -H tcp://127.0.0.1:2375 images      # 403 (IMAGES disabled)
docker -H tcp://127.0.0.1:2375 rm <id>     # 405 (READONLY=true)
```

## Build

```sh
go test ./...
go build ./...
docker buildx bake
```

`docker-bake.hcl` declares every published tag and platform, so one
command produces all of them. `REGISTRY`, `VERSION` and `GIT_SHA` are
bake variables — override any of them from the environment
(`VERSION=v1.1.0 docker buildx bake`). `GIT_SHA` defaults to `dev`, so a
local bake needs nothing; the workflow passes `github.sha`. `LATEST`
(default `false`) adds the `:latest` tags; `release.yml` sets it to `true`
only when the tag being released is the highest `v*` tag.

| Target | Base | Tags |
| --- | --- | --- |
| `docker-socket-proxy` | `gcr.io/distroless/static-debian13` | `:${VERSION}`, `:${GIT_SHA}`, `:latest` (highest release only) |
| `docker-socket-proxy-debug` | `…:debug` (busybox shell for `docker exec`) | same, `-debug` suffixed |

The builder image is `golang:1.27-bookworm` with `GOTOOLCHAIN=auto`, so the
`go` line in `go.mod` is the minimum Go version: a newer `go.mod` makes the
build fetch that toolchain, a newer image patch is used as is. No Dockerfile
edit on a Go patch bump.

Platforms: `linux/amd64`, `linux/arm64`, `linux/arm/v7`. The build stage
runs on `$BUILDPLATFORM` and cross-compiles with `CGO_ENABLED=0`, so
adding a platform to `docker-bake.hcl` is enough — no emulation.

Both images are built from the same build stage and ship an identical
binary; the final `FROM $BASE_IMAGE` is the only difference.

To build a single image locally:

```sh
docker buildx bake docker-socket-proxy \
  --set '*.platform=linux/arm64' --set '*.output=type=docker'
```

## Images

Releases are cut from `release/X.Y` branches; `main` never releases.

`ci.yml` runs on pull requests to `main` and `release/**`: `go vet`,
`go test ./...` and a build of all three platforms that publishes nothing;
`ci-ok` is the single required check. `auto-tag.yml` runs on every
push to `release/**`: a new `release/X.Y` gets `vX.Y.0`, an existing line
gets the next patch — only when `*.go` (not tests), `go.mod`, `go.sum`,
`Dockerfile` or `docker-bake.hcl` changed since the line's last tag;
Renovate merges wait until the branch's other Renovate PRs settle.
`release.yml` runs on the pushed `v*` tag: a Trivy scan of the image, a
bake that publishes to `ghcr.io/m-s-software-engineering-ag/docker-socket-proxy-go` with
`VERSION` taken from the tag name, then a GitHub release with notes since
the previous tag. `renovate.yml` keeps `main` current and sends only
security fixes to `release/*` (automerge). `daily-scan.yml` runs Trivy
on `main` and every `release/*` branch every morning.

`:latest` and the GitHub "Latest" release move only when the released tag
is the highest one, so a patch on an older line never rolls them back.
`:${VERSION}` and the full commit-sha tag never move — pin to either when
a deployment must keep running exactly what it rolled out with.
