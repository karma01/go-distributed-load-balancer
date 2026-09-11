# go-distributed-load-balancer

A round-robin HTTP load balancer written in Go using only the standard library. It fans incoming requests across a pool of backend servers, skips any backend that is unreachable, and continuously re-checks dead backends so they rejoin the rotation on their own.

No dependencies. No config files. One `main.go`.

## Architecture

```mermaid
flowchart LR
    C["Clients"]

    subgraph LB["Load Balancer :8080"]
        H["HTTP Handler"]
        P["ServerPool<br/>round-robin cursor"]
        HC["HealthCheck goroutine<br/>10s ticker"]
    end

    B1["Backend A :8081<br/>ALIVE"]
    B2["Backend B :8082<br/>ALIVE"]
    B3["Backend C :8083<br/>DOWN"]

    C --> H
    H --> P
    HC -. "SetAlive true/false" .-> P

    P -- "ReverseProxy" --> B1
    P -- "ReverseProxy" --> B2
    P -. "skipped" .-> B3

    HC -. "TCP dial 2s" .-> B1
    HC -. "TCP dial 2s" .-> B2
    HC -. "TCP dial 2s" .-> B3

    classDef lbBox fill:#1e293b,stroke:#38bdf8,stroke-width:2px,color:#f1f5f9
    classDef alive fill:#064e3b,stroke:#10b981,stroke-width:2px,color:#ecfdf5
    classDef dead fill:#450a0a,stroke:#ef4444,stroke-width:2px,color:#fef2f2
    classDef client fill:#312e81,stroke:#818cf8,stroke-width:2px,color:#eef2ff

    class H,P,HC lbBox
    class B1,B2 alive
    class B3 dead
    class C client
```

## How a request is routed

```mermaid
sequenceDiagram
    autonumber
    participant C as Client
    participant LB as Handler :8080
    participant SP as ServerPool
    participant B as Backend

    C->>LB: GET /
    LB->>SP: GetNextPeer()

    loop up to len(backends) attempts
        SP->>SP: NextIndex() — (current+1) % n
        SP->>SP: IsAlive()? (RLock)
    end

    alt a live backend was found
        SP-->>LB: *Backend
        LB->>B: ReverseProxy.ServeHTTP
        B-->>C: 200 OK
    else every backend is down
        SP-->>LB: nil
        LB-->>C: 503 All Servers are down
    end
```

## How it works

| Piece | Role |
| --- | --- |
| `Backend` | One upstream server: parsed URL, an `Alive` flag guarded by an `RWMutex`, and its own `httputil.ReverseProxy`. |
| `ServerPool` | Holds the backends and the round-robin cursor. `NextIndex()` advances `(current + 1) % len(backends)` under a mutex. |
| `GetNextPeer()` | Walks the ring at most `n` times looking for a backend where `IsAlive()` is true. Returns `nil` if the whole pool is down. |
| `HealthCheck()` | Runs in its own goroutine on a 10-second ticker. TCP-dials each backend host with a 2-second timeout and flips `Alive` accordingly. |

Read locks for the hot path (`IsAlive`), write locks only when a health check changes state — so request routing never blocks behind a health probe.

## Running it

The balancer listens on `:8080` and expects backends on `:8081`, `:8082`, and `:8083`.

```bash
go run main.go
```

Spin up three throwaway backends in separate terminals to watch it work:

```bash
# terminal 1
python3 -m http.server 8081
# terminal 2
python3 -m http.server 8082
# terminal 3
python3 -m http.server 8083
```

Then send some traffic:

```bash
for i in $(seq 1 6); do curl -s localhost:8080 > /dev/null; done
```

The balancer logs each hop:

```
Forwarding request to http://localhost:8081
Forwarding request to http://localhost:8082
Forwarding request to http://localhost:8083
Forwarding request to http://localhost:8081
```

Kill one backend and, within 10 seconds, the health checker notices and takes it out of the rotation:

```
Starting health check...
Backend is down: localhost:8082
```

Bring it back and it rejoins automatically on the next tick.

## Configuring backends

The pool is defined in `main.go`:

```go
serverList := []string{
    "http://localhost:8081",
    "http://localhost:8082",
    "http://localhost:8083",
}
```

Edit the slice to point at your own upstreams.

## Requirements

Go 1.27 or newer. Nothing else.

## Roadmap

- [ ] Backend list from CLI flags or a config file instead of a hardcoded slice
- [ ] Weighted and least-connections strategies alongside round-robin
- [ ] HTTP health checks (`GET /health`) rather than a bare TCP dial
- [ ] Retry the next peer on proxy error via `ReverseProxy.ErrorHandler`
- [ ] Structured logging and a `/metrics` endpoint
- [ ] Graceful shutdown on SIGTERM
