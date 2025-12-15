# devtest

A `T` interface wrapper for `testing.TB` to provide test conveniences.
As well as a `P` interface for package-level testing,
to enhance `TestMain(m testing.M)` or build some test-like Go programs.

## Usage

See [`run_example_test.go`](./run_example_test.go).

## Fork

This is a fork of [optimism devtest](https://github.com/ethereum-optimism/optimism/tree/develop/op-devstack/devtest),
a test utils library once written for OP-Stack.

Changes:
- No `assertify/require`, `t.Must` instead
- No hardcoded env vars, `context` usage instead
- No strict `*testing.T` dependency, `testing.TB` instead
- No `Gate` for test-skips
- Use improved logger package
- Add new `Output() io.Writer` to `T`
- Embed `testing.TB` into:
  - `testingT` for accurate `t.Helper()` stack inspection
  - `T` for extended interface
- Add `Run()` for managed `P` execution and shutdown using `runtime.Goexit` for go-routine
  to close the routine (with defers) gracefully, instead of a panic or other shortcut way.
- Improved error handling

## License

MIT License, see [LICENSE](./LICENSE) file.
