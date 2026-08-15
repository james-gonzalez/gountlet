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

```sh
./gountlet                  # interactive prompt (terminal) or run everything (piped/scripted)
./gountlet -cpu -mem        # only CPU and memory, no prompt
./gountlet -json            # machine-readable output, no prompt
./gountlet -duration 5s     # each timed benchmark runs for 5s instead of 3s
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
loopback self-test.

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
platform toolchain and loader. The Windows leg (MinGW-w64 + a Vulkan import
lib generated from the LunarG SDK) is the newest/least-proven part of the
pipeline; watch its CI run first if you touch it.
