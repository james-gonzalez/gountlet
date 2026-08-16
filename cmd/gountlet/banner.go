package main

// banner is shown above the interactive prompt only — flag-driven and
// piped runs stay clean/parseable. Keep this in sync with
// assets/banner.txt; go:embed can't reach outside this package's own
// directory, so it's duplicated here rather than embedded from there.
const banner = `██░░▒▒▓▓██
██           gountlet
██    ▒▒▓▓██   cpu · mem · disk · net · gpu
██        ██
██░░▒▒▓▓██
`
