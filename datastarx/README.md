# datastarx

Minimal Server-Sent-Events helpers for driving [Datastar](https://data-star.dev)
responses from `net/http`.

It's intentionally small: a correct SSE writer plus thin helpers for Datastar's
patch-elements / patch-signals events. It is **not** a full Datastar protocol
wrapper — for that, use the official Datastar Go SDK.

## Install

```sh
go get github.com/a3yko/kit/datastarx
```

## Usage

```go
func liveCount(w http.ResponseWriter, r *http.Request) {
    sse, err := datastarx.NewSSE(w)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    // Patch DOM elements (Datastar merges by element id):
    sse.PatchElements(`<div id="count">3</div>`)

    // Or patch the signal store:
    sse.PatchSignals(`{"count": 3}`)

    // Generic event (any name + data lines), if you need it:
    sse.Send("datastar-patch-elements", "elements <div id=\"x\">hi</div>")
}
```

`NewSSE` sets the right headers (`text/event-stream`, `no-cache`,
`X-Accel-Buffering: no` so nginx & friends don't buffer the stream) and uses
`http.ResponseController`, so it keeps working when middleware wraps the
`ResponseWriter` (a bare `http.Flusher` assertion would silently fail there).

## Notes

- An `SSE` is bound to one request and is **not safe for concurrent use**.
- The Datastar **event names are version-sensitive** — they were renamed across
  releases (`merge-*` → `patch-*`). They're constants in `datastar.go`; update
  them if you upgrade Datastar, or drop down to `Send` with the names your
  version expects.
