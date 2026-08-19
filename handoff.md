# Handoff — Go port of `worker-postgres` (tshark-streaming-ingestion)

**Date:** 2026-08-11
**Repo:** `/Users/rafael/Documents/tshark-streaming-ingestion` (branch `main`, last commit `33401f2`)
**Plan (authoritative, read this first):**
`/Users/rafael/.copilot/session-state/f11c342a-1cae-40b9-9184-96c8c57b6d1a/plan.md`

The plan file holds the full decision table, the gate criterion, findings F1–F4, the
per-file change log, and the staged todo list. **Do not re-derive any of it — read it.**
This document only covers what is *not* in the plan.

---

## One-paragraph orientation

The user asked to "refactor services into Golang to optimize RAM usage and speed."
A `/grill-me` session tested that premise against evidence and substantially narrowed
it. Speed is explicitly a non-goal (the PRD targets 100 pps and says so). RAM is the
stated driver, but the measured headroom is ~8.6% of stack footprint. The work was
therefore staged behind a measurement gate, and Stage 0 (fix Python defects, build a
black-box oracle, establish a baseline) was partially executed. Stage 0 surfaced four
findings, three of which are Python *defects* that Go would not have fixed, and one
(F4) which currently makes the gate **unmeasurable**. No Go service code has been
written yet.

## Critical state warnings

1. **Everything is uncommitted, on `main`, with no branch.** 14 modified files plus
   two untracked paths (`services/worker-postgres/tests/test_backpressure.py`,
   `tools/`). Nothing is committed. First action should probably be to create a branch
   and commit, or the work is one `git checkout` from gone.
2. **The todo database does not survive the session.** The 25 todos live in this
   session's SQLite DB. A fresh agent will not see them. The durable copy is the
   "Todos" and "Progress" sections of `plan.md`. Re-seed from there if you want
   tracking.
3. **The gate cannot currently be evaluated.** See F4 in the plan. Any soak run right
   now measures a 24 pps spawn-bound workload, not the 100 pps steady state specified.
   `s0-baseline` is blocked on `s0-fix-replay-spawn`.
4. **No trustworthy baseline exists**, so nothing measured so far either justifies or
   refutes the Go port. The user has not yet seen a valid number.

## What the user last asked

The user interrupted implementation with *"don't implement all changes, just plan for
now."* Implementation was stopped and findings were written into `plan.md` instead.
The open question put to them, still unanswered:

> Commit Stage 0 as-is, or continue planning F4's fix in more detail first?

**Do not resume broad implementation without an answer.** The user has twice steered
toward planning over execution.

## Immediate next actions (in order)

1. Get the user's answer on commit-vs-plan (above).
2. `s0-fix-metrics-init` — small, self-contained, fixes two already-broken Grafana
   panels. Good first commit.
3. `s0-fix-replay-spawn` — the blocker. Spawn `tshark` once, cache decoded ek lines,
   loop the cache. Until this lands nothing downstream is measurable.
4. `s0-baseline` — only meaningful after step 3.
5. Stage 1 (the actual Go worker) only after a real baseline exists.

## Environment facts (verified this session, don't re-check)

- Go **1.26.5** installed via Homebrew at `/usr/local/go/bin` — **not on the default
  `PATH`**. Prefix Go commands with `export PATH="/usr/local/go/bin:$PATH"`.
- Full stack is running in Docker: `postgres` (TimescaleDB pg18), `mosquitto`,
  `worker-postgres`, `ingestor-pcap`, `prometheus`, `grafana`, `cadvisor`.
- Metrics are now published to the host: worker `:8001`, ingestor `:8002`
  (container-internal port remains 8000; Prometheus scrapes over the docker network).
- Python tests: `poetry run pytest` → **34 passing**. Previously collected zero.
- Go harness: `cd tools/parity && go test ./...` → 11 passing, `go vet` clean.
- Real ek field naming is underscored (`ip_ip_src`, `frame_frame_len`) and `timestamp`
  is **milliseconds since epoch as a string**. Several original tests encoded a
  fictional dotted format; they were rewritten.
- `data/pcap/sample.pcap` holds **5 packets**. `make fetch-sample` pulls `http.cap`,
  which is also tiny — it does not solve F4.

## The parity harness (`tools/parity`)

New Go module, the intended oracle for the port. Black-box by design: it asserts only
on Postgres rows and the `/metrics` endpoint, never on internals, so it validates the
Python and Go workers identically.

```
parity verify  -a packets_py -b packets_go   # content checksum diff between two tables
parity soak    -metrics URL [-duration 30m]  # the Stage 1 RSS/dead-letter/buffer gate
parity drain   -table packets                # block until row count settles
parity metrics -metrics URL                  # assert the Grafana metric contract
```

Design points worth knowing before modifying it:
- Row checksums are **order-independent** (sum of per-row md5). MQTT QoS1 gives no
  cross-batch ordering guarantee, and many packets share a millisecond timestamp, so
  ordering by `ts` is not stable.
- Payload JSONB is checksummed **separately** from the shaped columns, so a mismatch
  immediately tells you whether the divergence is in shaping or in raw payload
  preservation (the re-marshalling trap — see the plan's hard constraints).
- `verify` and `soak` deliberately **fail on empty/idle input** rather than passing
  vacuously. Preserve that; it is what makes a green result meaningful.

## Judgement notes for the next agent

- The user responds well to being shown measurements that contradict their premise —
  every recommendation accepted this session was one grounded in a number pulled from
  their own running stack or their own PRD. Verify before asserting.
- The user overrode one recommendation: they chose to **delete** the Python worker
  after cutover rather than keep it as a rollback oracle. That means the parity run
  (`s1-parity-run`) is the *only* chance to validate the port and must pass before
  deletion. This ordering is already encoded in the plan's dependencies — respect it.
- Be honest that this project's stated payoff is ~25 MiB on one service. That framing
  was accepted, not resisted. Don't quietly inflate the justification.
- Regression tests written this session were explicitly verified to **fail against the
  original buggy code** before being accepted. Hold new tests to the same standard.

## Suggested skills

- **`test-driven-development`** — F1's fix was only trusted because the regression test
  was proven to fail against the buggy original. Apply the same discipline to
  `s0-fix-metrics-init` and `s0-fix-replay-spawn`, and to every ported Go component.
- **`incremental-implementation`** — Stage 1 touches many files; the plan already
  sequences it (parity-first, `CopyFrom` only afterwards) specifically so failures stay
  bisectable. This skill keeps that discipline.
- **`git-workflow-and-versioning`** — everything is uncommitted on `main`. Needed
  immediately to get the work onto a branch with sensible commit boundaries.
- **`debugging-and-error-recovery`** — for F4 (spawn-bound replay) and the unexplained
  ingestor RSS growth to 88 MiB, both of which need root-cause work rather than
  guessing.
- **`performance-optimization`** — for capturing the Stage 0 baseline and evaluating
  the gate correctly once F4 is fixed.
- **`grill-me`** — if the user proposes expanding scope (e.g. porting the ingestor
  early, or skipping the gate), re-run the interview rather than complying. It has
  already prevented one unjustified rewrite in this project.

## Sensitive data

No credentials are included here. Note that `POSTGRES_DSN` defaults in the compose
files and `tools/parity` contain **local development-only** Postgres credentials that
are already committed to the repo; they are not production secrets, but do not
propagate them into new files, logs, or issue text.
