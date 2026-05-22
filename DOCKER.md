# Docker Deployment Guide for go-atomos

## Overview

go-atomos supports Docker-native operation: console logging, signal-based shutdown,
environment variable configuration, and health checks. This guide covers containerizing
an Atomos application from development to production.

## Quick Start

Minimal Dockerfile for an Atomos app:

```dockerfile
FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /app ./cmd/myapp

FROM gcr.io/distroless/static-debian12
COPY --from=build /app /app
COPY config.yaml /etc/atomos/config.yaml
ENTRYPOINT ["/app", "--config", "/etc/atomos/config.yaml"]
```

Build and run:

```bash
docker build -t my-atomos-app .
docker run --rm my-atomos-app
docker logs -f <container-id>   # all application output visible
docker stop <container-id>      # graceful shutdown via SIGTERM
```

## Configuration

### Environment Variables

All `ATOMOS_*` variables override YAML config values. Variables take precedence.

| Variable | Type | Description |
|----------|------|-------------|
| `ATOMOS_COSMOS` | string | Cosmos cluster name |
| `ATOMOS_NODE` | string | Node name within the cosmos |
| `ATOMOS_LOG_LEVEL` | string | debug / info / warn / error / fatal |
| `ATOMOS_LOG_PATH` | string | Log directory (ignored when logging to stdout) |
| `ATOMOS_LOG_STDOUT` | bool | Force console logging (true / 1) |
| `ATOMOS_ETCD_ENDPOINTS` | string | Comma-separated etcd endpoints |
| `ATOMOS_STANDALONE` | bool | Skip daemon forking (true / 1) |

### YAML Config

```yaml
cosmos: my-cosmos
node: node-1
log-level: info
log-path: /var/log/atomos
log-max-size: 10485760
log-std: true        # log to stdout/stderr (activated)
run-path: /var/run/atomos
etc-path: /etc/atomos

enable-cluster:
  enable: true
  etcd-endpoints:
    - etcd-0.etcd:2379
    - etcd-1.etcd:2379
  optional-ports:
    - 50051
    - 50052

enable-elements:
  - MyElement
```

### Configuration Precedence

1. CLI flags (`--config`, `--standalone`)
2. Environment variables (`ATOMOS_*`)
3. YAML config file
4. Auto-detection (Docker environment)

## Logging

### Console Mode (Recommended for Docker)

In Docker, the framework auto-detects the container environment and logs to stdout/stderr.
You can also force it explicitly:

```bash
# via env var
docker run -e ATOMOS_LOG_STDOUT=true my-atomos-app

# via YAML
# config.yaml:
#   log-std: true
```

Log levels are written to stdout (access/info/debug) or stderr (error/fatal).
`docker logs` captures both streams by default.

### File Mode (Legacy)

Outside Docker, logs go to files by default at `{log-path}/access.*.log` and
`{log-path}/error.*.log` with automatic rotation and cleanup. You can still use
this inside Docker if you mount a volume:

```bash
docker run -v /host/logs:/var/log/atomos -e ATOMOS_LOG_STDOUT=false my-atomos-app
```

## Signal Handling & Graceful Shutdown

The framework handles these signals for clean shutdown:

| Signal | Behavior |
|--------|----------|
| `SIGTERM` | Graceful shutdown (Docker default) |
| `SIGINT` | Graceful shutdown (Ctrl+C) |
| `SIGHUP` | Graceful shutdown |
| `SIGQUIT` | Graceful shutdown |

The shutdown sequence:

1. Signal received → `mainScript.OnShutdown()` called
2. Elements killed in reverse spawn order
3. Atoms within each element killed concurrently
4. Auto-data persistence saves state (if implemented)
5. etcd lease released, gRPC server stopped
6. Process exits

Docker sends `SIGTERM` and waits 10 seconds (default) before `SIGKILL`.
Adjust this per your app's shutdown duration:

```dockerfile
STOPSIGNAL SIGTERM
# or at runtime:
# docker run --stop-timeout 30 my-atomos-app
```

## Health Checks

### Using the Framework API

```go
// In your main script or health endpoint:
func (s *myMainScript) OnStartUp(local *atomos.CosmosProcess) *atomos.Error {
    // Register a periodic health reporter
    local.Self().Task().AddAfter(5*time.Second, func(taskID uint64) {
        if local.IsHealthy() {
            // all good
        }
    })
    return nil
}
```

### Docker HEALTHCHECK

```dockerfile
HEALTHCHECK --interval=5s --timeout=3s --retries=3 \
    CMD ["/app", "--health-check"]
```

For a dedicated health check binary or command, call `SharedCosmosProcess().IsHealthy()`
and exit 0 (healthy) or 1 (unhealthy).

### Kubernetes Probes

```yaml
livenessProbe:
  exec:
    command:
    - /app
    - --health-check
  initialDelaySeconds: 10
  periodSeconds: 5
readinessProbe:
  exec:
    command:
    - /app
    - --health-check
  initialDelaySeconds: 5
  periodSeconds: 3
```

## Clustering with etcd

### Single-Node Development

```bash
docker network create atomos-net

docker run -d --name etcd --network atomos-net \
    quay.io/coreos/etcd:v3.5 \
    etcd --listen-client-urls http://0.0.0.0:2379 \
         --advertise-client-urls http://etcd:2379

docker run --rm --network atomos-net \
    -e ATOMOS_ETCD_ENDPOINTS=etcd:2379 \
    my-atomos-app
```

### Multi-Node Production

Use a StatefulSet for ordered deployment:

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: atomos-node
spec:
  serviceName: atomos
  replicas: 3
  template:
    spec:
      containers:
      - name: atomos
        image: my-atomos-app:latest
        env:
        - name: ATOMOS_NODE
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: ATOMOS_COSMOS
          value: production
        - name: ATOMOS_ETCD_ENDPOINTS
          value: etcd-0.etcd:2379,etcd-1.etcd:2379,etcd-2.etcd:2379
        - name: ATOMOS_LOG_STDOUT
          value: "true"
        ports:
        - containerPort: 50051
          name: grpc
        livenessProbe:
          exec:
            command: ["/app", "--health-check"]
          initialDelaySeconds: 15
          periodSeconds: 10
```

### Port Requirements

Each Atomos node needs one available port from the `optional-ports` list for gRPC
inter-node communication. In Kubernetes, use a headless service for pod-to-pod gRPC:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: atomos
spec:
  clusterIP: None
  selector:
    app: atomos-node
  ports:
  - port: 50051
    name: grpc
```

## Forking Behavior

The framework normally forks a daemon child process. In Docker this is unnecessary
and is automatically skipped (detected via `/.dockerenv`). You can also force it:

```bash
docker run -e ATOMOS_STANDALONE=true my-atomos-app
# or via flag:
docker run my-atomos-app --standalone
```

## PID Files

PID files are automatically skipped in Docker. Outside Docker, they prevent duplicate
instances on the same host. If you mount a shared volume, consider disabling PID files
by setting a container-specific `run-path`:

```yaml
run-path: /tmp/atomos-run   # ephemeral, unique per container
```

## Resource Limits

```yaml
# Kubernetes
resources:
  requests:
    memory: "256Mi"
    cpu: "500m"
  limits:
    memory: "1Gi"
    cpu: "2"
```

```bash
# Docker
docker run --memory 1g --cpus 2 my-atomos-app
```

The framework's mailbox model means each Atom/Element has its own goroutine.
Memory usage scales with active Actors × average state size. CPU usage is
proportional to message throughput.

## Troubleshooting

### Container exits immediately

Check that `--config` points to a valid YAML file inside the container.
Verify the YAML file was copied in the Dockerfile.

### No output in docker logs

Ensure `ATOMOS_LOG_STDOUT=true` or `log-std: true` is set. Without this,
logs go to files and `docker logs` sees nothing.

### docker stop takes too long

The framework stops Elements in reverse spawn order and waits for each Atom to halt.
If `StopTimeout` is high, increase Docker's stop timeout:

```bash
docker run --stop-timeout 60 my-atomos-app
```

### PID file error on restart

If mounting a persistent volume for `run-path`, stale PID files from previous
container runs can cause "app is already running" errors. Solutions:

1. Use a tmpfs for run-path: `--tmpfs /var/run/atomos`
2. Set run-path to `/tmp` (ephemeral per container)
3. Set `ATOMOS_STANDALONE=true` and `ATOMOS_LOG_STDOUT=true`

### Inter-node gRPC connection failures

Verify that `optional-ports` includes a port that is actually exposed and reachable.
In Kubernetes, use a headless service and ensure network policies allow pod-to-pod
traffic on the gRPC port.

## Example: Complete Docker Compose Stack

```yaml
version: "3.8"
services:
  etcd:
    image: quay.io/coreos/etcd:v3.5
    command:
      - etcd
      - --listen-client-urls=http://0.0.0.0:2379
      - --advertise-client-urls=http://etcd:2379

  atomos-1:
    build: .
    environment:
      ATOMOS_COSMOS: dev
      ATOMOS_NODE: node-1
      ATOMOS_ETCD_ENDPOINTS: etcd:2379
      ATOMOS_LOG_STDOUT: "true"
    ports:
      - "50051:50051"
    depends_on:
      - etcd

  atomos-2:
    build: .
    environment:
      ATOMOS_COSMOS: dev
      ATOMOS_NODE: node-2
      ATOMOS_ETCD_ENDPOINTS: etcd:2379
      ATOMOS_LOG_STDOUT: "true"
    depends_on:
      - etcd
```
