## Workflow Orchestration

### 1. Plan Node Default
- Enter plan mode for ANY non-trivial task (3+ steps or architectural decisions)
- If something goes sideways, STOP and re-plan immediately - don't keep pushing
- Use plan mode for verification steps, not just building
- Write detailed specs upfront to reduce ambiguity

### 2. Subagent Strategy
- Use subagents liberally to keep main context window clean
- Offload research, exploration, and parallel analysis to subagents
- For complex problems, throw more compute at it via subagents
- One task per subagent for focused execution

### 3. Self-Improvement Loop
- After ANY correction from the user: update `tasks/lessons.md` with the pattern
- Write rules for yourself that prevent the same mistake
- Ruthlessly iterate on these lessons until mistake rate drops
- Review lessons at session start for relevant project

### 4. Verification Before Done
- Never mark a task complete without proving it works
- Diff behavior between main and your changes when relevant
- Ask yourself: "Would a staff engineer approve this?"
- Run tests, check logs, demonstrate correctness

### 5. Demand Elegance (Balanced)
- For non-trivial changes: pause and ask "is there a more elegant way?"
- If a fix feels hacky: "Knowing everything I know now, implement the elegant solution"
- Skip this for simple, obvious fixes - don't over-engineer
- Challenge your own work before presenting it

### 6. Autonomous Bug Fixing
- When given a bug report: just fix it. Don't ask for hand-holding
- Point at logs, errors, failing tests - then resolve them
- Zero context switching required from the user
- Go fix failing CI tests without being told how

## Task Management

1. **Plan First**: Write plan to `tasks/todo.md` with checkable items
2. **Verify Plan**: Check in before starting implementation
3. **Track Progress**: Mark items complete as you go
4. **Explain Changes**: High-level summary at each step
5. **Document Results**: Add review section to `tasks/todo.md`
6. **Capture Lessons**: Update `tasks/lessons.md` after corrections

## Core Principles

- **Simplicity First**: Make every change as simple as possible. Impact minimal code.
- **No Laziness**: Find root causes. No temporary fixes. Senior developer standards.
- **Minimal Impact**: Changes should only touch what's necessary. Avoid introducing bugs.

## Project: docker-socket-proxy

Go reverse proxy that fronts `/var/run/docker.sock` and exposes the
Docker Engine REST API (spec: v1.54 — https://docs.docker.com/reference/api/engine/version/v1.54/)
on a TCP port with per-section allowlists. Ships as a distroless image
built with `docker buildx bake`. Stdlib only (no third-party deps).

### Architecture

- `main.go` — env parse, slog setup, startup probe, http.Server + signal
  handling.
- `internal/proxy/router.go` — section table (linuxserver-compatible env
  names), version-prefix strip, allow / readonly / unknown decision.
- `internal/proxy/probe.go` — 10s reachability probe with retry; logs
  `socket.retry` per attempt, `socket.ready` on success.
- `internal/proxy/proxy.go` — `httputil.ReverseProxy` + unix socket
  transport + audit middleware (preserves Hijacker/Flusher for
  attach/exec/build streams).
- `internal/audit/audit.go` — capped req/resp body capture, stream
  detection.

### Env vars

Full matrix in `README.md`. API gates default `false`. `READONLY`
default `true`. Truthy parse: `true` only (case-insensitive). Anything
else = false. **Not `1`/`0`.**

### Verification

```sh
go test ./...
go build ./...
docker buildx bake --print
docker buildx bake docker-socket-proxy \
  --set '*.platform=linux/arm64' --set '*.output=type=docker'
```

### Build

- `docker-bake.hcl` is the source of truth for tags and platforms
  (`linux/amd64`, `linux/arm64`, `linux/arm/v7`); the Dockerfile's
  `BASE_IMAGE` arg is the only difference between the normal and
  `-debug` targets.
- Releases come from `release/X.Y` branches only (same model as
  `env2yml`): `pr.yml` (vet + test + bake, `ci-ok` gate),
  `auto-release.yml` (push to `release/**` → `vX.Y.0` or next patch, only
  if `*.go`/`go.mod`/`go.sum`/`Dockerfile`/`docker-bake.hcl` changed;
  waits for pending Renovate PRs), `release.yml` (on `v*` tag: Trivy,
  bake + push to `ghcr.io/basecrusher/docker-socket-proxy-go`, GitHub
  release), `renovate.yml` (release branches: security only),
  `daily-scan.yml` (main + every `release/*`).
- `:latest` / GitHub "Latest" only for the highest tag: `release.yml`
  passes bake `LATEST=true|false`, default `false`.
- Builder is `golang:1.27-bookworm` + `GOTOOLCHAIN=auto`; the `go` line in
  `go.mod` is the floor (fetched if newer than the image), so no Dockerfile
  edit on Go bumps.
- Any change to the build must also update `README.md` (tags, targets,
  platforms).

### Security stance

- Default-deny on every API gate; `READONLY=true` default.
- Distroless `gcr.io/distroless/static-debian13`, no shell.
- **Runs as root (uid/gid 0)** — deliberate: the docker socket is
  `root:docker` and no unprivileged uid can read it. This is the only
  container in the fleet that gets to be root; everything else is
  rootless and reaches Docker through this proxy over TCP.
- Mount the docker socket **read-only** when running this container.
- No TLS in v1 — bind to a private docker network or terminate TLS in a
  sidecar.
