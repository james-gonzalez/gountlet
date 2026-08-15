# gountlet

Cross-platform (Linux/macOS/Windows) performance benchmark: CPU (single-core
and multi-core), memory, disk, network, and GPU compute — one static-ish Go
binary, no external dependencies at runtime beyond your GPU's Vulkan loader.

## Build

```sh
go build ./cmd/gountlet
```

Requires a C compiler (`CGO_ENABLED=1`, the default) to build the GPU
benchmark. Build natively on each target OS — Linux, macOS, and Windows each
need their own toolchain (gcc/clang, Xcode command line tools, or
MSVC/mingw-w64 respectively) since cgo cross-compilation isn't practical here.
If you build with `CGO_ENABLED=0`, everything else still works; the GPU
benchmark just reports that it needs cgo.

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

Run `gountlet` bare in a terminal with no flags and it walks you through
picking benchmarks, duration, and output format interactively instead of
requiring a pile of flags. Pass any flag (or pipe/redirect stdin) and it
skips the prompt and behaves like a normal CLI — that's what scripts and CI
should do.

Flags: `-cpu -mem -disk -net -gpu -all` select which benchmarks run (default:
all). `-disk-path <dir>` picks where the disk benchmark's temp file goes
(default OS temp dir). `-net-target host:port` points the network benchmark
at a `gountlet -net-serve` instance on another machine instead of the default
loopback self-test. `-stress` runs each timed benchmark for 5 minutes instead
of the default 3 seconds, for burn-in/thermal-throttling checks under
sustained load rather than a quick snapshot; an explicit `-duration` overrides it.

```sh
# on machine A
./gountlet -net-serve :9494

# on machine B
./gountlet -net -net-target machineA:9494
```

## What each benchmark measures

- **cpu** — SHA-256 hashing throughput (MH/s), single goroutine vs. one
  goroutine per logical core, plus the resulting scaling factor.
- **memory** — sequential read/write bandwidth (GB/s) over a 512 MiB buffer,
  plus random-access latency (ns/op) via cache-line-strided pointer chasing.
- **disk** — sequential write/read throughput (MB/s) and 4K random
  read/write IOPS against a temp file. No O_DIRECT (kept portable across all
  three OSes), so repeated reads may be inflated by the OS page cache.
- **network** — TCP upload/download throughput (Mbps). Self-contained by
  default (spins up a loopback server); point it at another machine's
  `-net-serve` for a real link test.
- **gpu** — dispatches a Vulkan compute shader doing a large number of
  fused multiply-adds per element and reports GFLOPS, plus the selected
  device's name. Needs the platform's Vulkan runtime loader installed
  (`libvulkan.so.1` / `vulkan-1.dll` / `libvulkan.dylib` via MoltenVK on
  macOS) — no display or windowing system required.

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
  Gen3 / NVMe Gen4+ ranges. Read numbers carry an explicit page-cache
  caveat, since gountlet doesn't use O_DIRECT (see above) and a cached read
  can easily outrun the real device.
- **network** throughput is bucketed against standard Ethernet link classes
  (100Mbps/1GbE/2.5GbE/5GbE/10GbE+) — except for the default loopback
  self-test, which is labeled as such instead of pretending to be a real
  link measurement.
- **cpu** scaling gets an efficiency percentage against ideal linear
  scaling. The raw MH/s numbers don't get a fabricated "tier" — SHA-256
  throughput from this specific workload isn't a standardized benchmark
  with established reference ranges the way memory/disk/network are.
- **gpu** compute is labeled integrated vs. discrete (a real fact from the
  Vulkan device properties), not an absolute performance tier for the same
  reason as CPU.

## GPU benchmark internals

The Vulkan headers needed to build (`vulkan_core.h`, `vk_platform.h`, and the
`vk_video/*` headers it pulls in) are vendored under
`internal/bench/gpu/vk/include/` so building only requires the Vulkan
*runtime loader*, not the full Vulkan SDK. See
`internal/bench/gpu/vk/LICENSE` for their license (Khronos, Apache-2.0/MIT).

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
