# orchestrator

A local, open-source orchestrator that routes any task to the right agent or skill, runs it, and reports back with proof — not a confident guess.

Given a task, it selects a catalog entry, checks that entry's permission before doing anything, invokes it through an adapter, and prints an attributed report that names exactly which entry produced the result.

## Status

As of commit `aa747b7`. Five packages exist; all build and pass tests — `go build ./...` and `go test ./...` are both green (the counts below are from a fresh `go test ./... -count=1`):

| Stage | Package | Responsibility | Tests |
|-------|---------|----------------|-------|
| 1 | `catalog`  | manifest schema + directory loader          | 11 passing |
| 2 | `selector` | task → catalog entry selection (via Claude)  | 7 passing |
| 3 | `adapter`  | compliant HTTP adapter (GET invocation)      | 10 passing |
| 4 | `executor` | permission engine + adapter router           | 12 passing |
| 5 | `reporter` | attributed run report                        | 7 passing |

The five packages above, plus the `cmd/orchestrator` CLI, are the whole system today.

**Review status:** each stage went through an adversarial multi-agent review during development (a development aid, not a formal audit). Stage 5 (`reporter`) was reviewed this session with no confirmed defects (3 minor, non-blocking); however, the attribution fix included in this commit was applied *after* that review and is verified by tests only — so re-review of the committed `reporter`/`executor` change is **pending**.

## Build

```
go build ./...
```

To produce the CLI binary:

```
go build -o orchestrator ./cmd/orchestrator
```

## Test

```
go test ./...
```

## Run

The repo ships two seed manifests in `manifests/`: `weather` (a live Open-Meteo forecast, permission `auto`) and `claude-code-cli` (permission `ask`). Run the commands below from the repo root — the default manifest directory is `./manifests`.

`select` and `run` call Claude to choose an entry, so they need `ANTHROPIC_API_KEY` set. `catalog list` does not.

**`catalog list`** — load the manifest directory and list its entries (no API key needed):

```
$ ./orchestrator catalog list
NAME             TYPE      PERMISSION
claude-code-cli  delegate  ask
weather          delegate  auto
```

**`select`** — pick the entry best suited to a task (prints the entry name, or `no match`):

```
$ export ANTHROPIC_API_KEY=sk-ant-...
$ ./orchestrator select "what's the weather in Berlin right now?"
weather
```

**`run`** — the full pipeline: select, permission-check, invoke, report:

```
$ ./orchestrator run "current weather in Berlin"
Task:    current weather in Berlin
Entry:   weather
Outcome: success — "weather" ran (permission: auto)
Output:
{"current_weather":{"temperature":24.1,"windspeed":9.0, ...}}
When:    2026-07-02T00:00:00Z
```

(Example output. `weather` has permission `auto`, so it runs without a prompt. An entry with permission `ask` prompts for `y/n` approval first; `never` is rejected without running.)

## Architecture

```
catalog → selector → executor (permission-gated) → adapter → reporter
```

A manifest **catalog** defines the available entries; the **selector** picks one for the task; the **executor** enforces that entry's permission (`auto` / `ask` / `never`) and routes it; the **adapter** invokes it; the **reporter** prints an attributed result.

## License

MIT — see [LICENSE](LICENSE).
