<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/gountlet-mark-dark-bg.svg">
  <img src="assets/gountlet-mark-light-bg.svg" alt="gountlet" width="72" height="72">
</picture>

# gountlet

Cross-platform (Linux/macOS/Windows) performance benchmark: CPU (single-core
and multi-core), memory, disk, network, and GPU compute — one static-ish Go
binary with no required runtime dependencies; the GPU benchmark additionally
uses your system's Vulkan loader if one is installed, and simply reports
itself unavailable if not.

## Install

**macOS/Linux via Homebrew:**

```sh
brew tap james-gonzalez/gountlet
brew install gountlet
```

Installs a prebuilt binary from the latest [release](https://github.com/james-gonzalez/gountlet/releases) —
no Go or C toolchain needed. The formula lives in
[homebrew-gountlet](https://github.com/james-gonzalez/homebrew-gountlet) and
updates itself automatically on every release.

Otherwise, grab a prebuilt archive from the
[releases page](https://github.com/james-gonzalez/gountlet/releases) for
your platform, or build from source below.

## Build

```sh
go build ./cmd/gountlet
```

Requires a C compiler (`CGO_ENABLED=1`, the default) to build the GPU
benchmark — nothing else. The Vulkan headers are vendored under
`internal/bench/gpu/vk/include/`, and the Vulkan loader itself is loaded
dynamically at runtime (`dlopen`/`LoadLibrary`, not linked at build time —
see [GPU benchmark internals](#gpu-benchmark-internals)), so no Vulkan SDK
or loader package needs to be installed to build on any platform. Build
natively on each target OS — Linux, macOS, and Windows each need their own
C toolchain (gcc/clang, Xcode command line tools, or MinGW-w64
respectively) since cgo cross-compilation isn't practical here. If you
build with `CGO_ENABLED=0`, everything else still works; the GPU benchmark
just reports that it needs cgo.

A Vulkan loader only needs to be *installed* on whatever machine actually
*runs* the GPU benchmark — if it's missing, that one benchmark reports
itself unavailable and every other benchmark still runs normally (see
[GPU benchmark internals](#gpu-benchmark-internals)).

## Run

> **Windows:** release binaries aren't code-signed, so Windows may block
> them with *"An Application Control policy has blocked this file"* after
> downloading. Run `Unblock-File .\gountlet.exe` (or right-click → Properties
> → check "Unblock") to clear the Mark-of-the-Web that triggers this. If
> Smart App Control is enabled, it blocks unsigned exes outright regardless —
> there's no workaround short of a code-signing cert.

```sh
./gountlet                  # interactive prompt (terminal) or run everything (piped/scripted)
./gountlet -cpu -mem        # only CPU and memory, no prompt
./gountlet -json            # machine-readable output, no prompt
./gountlet -duration 5s     # each timed benchmark runs for 5s instead of 3s
./gountlet -stress          # each timed benchmark runs for 5m instead of 3s
```

Run `gountlet` bare in a terminal with no flags and it launches a full
graphical TUI: a setup screen to pick benchmarks/duration/paths (arrow keys
or j/k to move, space to toggle a checkbox, enter to edit a field or start),
then a live progress view, then a bar-chart results view (q to quit). The
progress view shows each benchmark's sub-tests as they run too — e.g. cpu's
`single-core-hash`/`multi-core-hash`/`hash-scaling` or disk's
`sequential-write`/`sequential-read`/`random-write`/`random-read` — appearing
and checking off one at a time instead of one opaque spinner per benchmark.
Pass any flag (or pipe/redirect stdin) and it skips all of that and behaves like
a normal CLI, printing a plain table — that's what scripts and CI should do.
If bubbletea can't run in the attached terminal for some reason, it falls
back automatically to a plain line-by-line text prompt rather than failing
outright.

Flags: `-cpu -mem -disk -net -gpu -all` select which benchmarks run (default:
all — passing any of these also skips straight to plain-text output, no TUI).
`-disk-path <dir>` picks where the disk benchmark's temp file goes (default
OS temp dir). `-net-target host:port` points the network benchmark at a
`gountlet -net-serve` instance on another machine instead of the default
loopback self-test. `-stress` runs each timed benchmark for 5 minutes instead
of the default 3 seconds, for burn-in/thermal-throttling checks under
sustained load rather than a quick snapshot; an explicit `-duration` overrides it.

`-tui` shows the same live progress + bar-chart results view as the bare
invocation, but for a flag-driven run — skips the setup screen since the
flags already answered those questions. `-html <path>` writes a
self-contained HTML report (bar charts, inline CSS, no JS) alongside
whatever else you asked for; works with or without `-tui`.

`-duration`/`-stress` apply to every benchmark, not just CPU: memory and
disk run at least one full pass over their fixed-size buffer/file and then
keep repeating (wrapping back to the start, so disk's on-disk footprint
stays capped at 1 GiB regardless of duration) until the time's up; GPU
calibrates and scales its dispatch to the requested duration; network already
worked this way. Since every phase gets the full duration, total run time
for `-all` is roughly (duration × ~12 phases) — `-stress -all` is a
~60-minute run by design, not a quick check.

```sh
# on machine A
./gountlet -net-serve :9494

# on machine B
./gountlet -net -net-target machineA:9494
```

## What each benchmark measures

- **cpu** — two workloads: SHA-256 hashing (integer/bitwise, MH/s) and dense
  matrix multiplication (floating point, GFLOPS). Each runs single-core and
  all-core for the full duration, plus a thread-scaling curve at
  intermediate power-of-2 thread counts (a short fixed 500ms sample each,
  just to show the curve's shape — not held to the same duration as the
  two endpoints either side of it).
- **memory** — sequential read/write bandwidth (GB/s) over a 512 MiB buffer,
  plus random-access latency (ns/op) via cache-line-strided pointer chasing.
- **disk** — sequential write/read throughput (MB/s) and 4K random
  read/write IOPS against a temp file. Reads bypass the OS page cache where
  the platform/filesystem supports it (O_DIRECT on Linux,
  `FILE_FLAG_NO_BUFFERING` on Windows, `F_NOCACHE` on macOS) so they reflect
  real device I/O rather than a cache hit; where that isn't possible (e.g.
  tmpfs, which has no underlying device to bypass to), it falls back to a
  normal cached read and the result says so.
- **network** — TCP upload/download throughput (Mbps). Self-contained by
  default (spins up a loopback server); point it at another machine's
  `-net-serve` for a real link test.
- **gpu** — dispatches a Vulkan compute shader doing a large number of
  fused multiply-adds per element and reports GFLOPS, plus the selected
  device's name. Needs the platform's Vulkan runtime loader installed
  (`libvulkan.so.1` / `vulkan-1.dll` / `libvulkan.dylib` via MoltenVK on
  macOS) — no display or windowing system required. If the loader isn't
  installed, only this benchmark fails (with a clear error); every other
  benchmark is unaffected. Timing uses GPU timestamp queries (pure
  on-device execution time) where the device supports them, falling back
  to CPU wall-clock — which also captures some driver/submission
  overhead — otherwise; the result's `timing` field says which was used.

## Result context and device info

Each benchmark also reports what hardware it ran against — CPU model and
physical/logical core count, installed RAM (+ type/speed where obtainable),
the disk device/filesystem/model backing the temp file, and the primary
network interface's name/MAC/link speed. Every field is best-effort: things
like RAM type/speed or a disk's model name often need elevated privileges or
platform tools that aren't always present, and are simply omitted rather
than shown as a guess when unavailable — Linux in particular usually can't
read RAM type/speed (`dmidecode`) without root, while Windows and macOS
typically can via WMI/`system_profiler` without any elevation.

Several metrics also get a short interpretive note in the table output
(and a `context` field in `-json`) — a classification against
well-established real-world ranges, not an invented precision score:

- **memory** bandwidth is bucketed low/moderate/high/very-high as a
  *single-thread* figure — a lone goroutine won't saturate a multi-channel
  controller the way a STREAM-style benchmark would, so don't read too much
  generational meaning into it. Random-access latency is compared against
  the ~50-120ns DRAM range, which holds roughly across DDR generations.
- **disk** throughput/IOPS are bucketed against HDD / SATA SSD / NVMe
  Gen3 / NVMe Gen4+ ranges. Read numbers only carry a page-cache caveat when
  the cache bypass actually fell back to a normal cached read (see above).
- **network** throughput is bucketed against standard Ethernet link classes
  (100Mbps/1GbE/2.5GbE/5GbE/10GbE+) — except for the default loopback
  self-test, which is labeled as such instead of pretending to be a real
  link measurement.
- **cpu** scaling (both workloads) gets an efficiency percentage against
  ideal linear scaling. The raw MH/s/GFLOPS numbers don't get a fabricated
  "tier" — neither this SHA-256 loop nor this naive (non-vectorized) matrix
  multiply is a standardized benchmark with established reference ranges
  the way memory/disk/network are.
- **gpu** compute is labeled integrated vs. discrete (a real fact from the
  Vulkan device properties), not an absolute performance tier for the same
  reason as CPU.

## GPU benchmark internals

The Vulkan headers needed to build (`vulkan_core.h`, `vk_platform.h`, and the
`vk_video/*` headers it pulls in) are vendored under
`internal/bench/gpu/vk/include/`. See `internal/bench/gpu/vk/LICENSE` for
their license (Khronos, Apache-2.0/MIT).

The Vulkan loader itself is **not** linked at build time — the binary
never has a hard `libvulkan.so.1`/`vulkan-1.dll`/`libvulkan.dylib`
dependency, so it always starts, on any machine, GPU loader present or
not. Instead, `Run()` calls `dlopen`/`LoadLibraryA` on the loader at the
start of the GPU benchmark and resolves every Vulkan function it needs
through `vkGetInstanceProcAddr`/`vkGetDeviceProcAddr` (the header is
compiled with `VK_NO_PROTOTYPES`, so nothing pulls in the real symbols at
link time). If the loader can't be found, `Run()` returns a normal failed
result for the `gpu` benchmark only — every other benchmark, and the
binary's ability to start at all, is unaffected. This is what fixes
`gountlet` failing to even launch (`error while loading shared libraries:
libvulkan.so.1: cannot open shared object file`) on machines like a
Raspberry Pi that have no Vulkan loader installed.

## CI/CD and releases

Every push/PR to `main` runs `.github/workflows/ci.yml`: `go vet`, `gofmt`,
`golangci-lint`, `govulncheck`, and a native build of every release target
(as a CI-only check, artifacts kept 14 days).

Pushes to `main` also run `.github/workflows/release.yml`: the same native
build matrix, then [semantic-release](https://semantic-release.gitbook.io/)
(via `cycjimmy/semantic-release-action`, config in `.releaserc.json`)
inspects [Conventional Commits](https://www.conventionalcommits.org/) since
the last release to decide whether/what to publish, and if so, tags a
GitHub Release with generated notes and attaches every platform's archive +
a `checksums.txt`.

Each build job (`.github/workflows/build.yml`, called by both workflows) is
a **native** build — Linux amd64/arm64, macOS arm64/amd64, Windows amd64 —
not a cross-compile, since the GPU benchmark's cgo+Vulkan code needs a real
platform toolchain and loader. The Windows leg (MinGW-w64, linked directly
against the LunarG SDK's `Lib\vulkan-1.lib`) is the newest/least-proven part
of the pipeline; watch its CI run first if you touch it.
