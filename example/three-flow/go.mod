module github.com/hstern/go-rar/example/three-flow

go 1.26

require github.com/hstern/go-rar v0.1.0

// During development inside this repository (and for contributors
// cloning a fresh checkout), build the example against the library
// source sitting two directories up rather than against the published
// v0.1.0 tag. Keeping the replace in the committed go.mod means
// `cd example/three-flow && go run .` works straight after a clone
// without an intermediate publish step — the example doubles as a
// fast smoke test of any in-progress library change.
replace github.com/hstern/go-rar => ../..
