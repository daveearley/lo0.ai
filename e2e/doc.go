// Package e2e tests the lo0 binary as a user runs it: a separate process
// with its own home directory, talking to a fake Ollama on loopback and to
// fake harnesses on its PATH, with every other connection caught by a
// recording proxy. The unit tests check each piece; these check that the
// pieces, wired together by cmd/lo0, keep the promises the README makes.
//
// The tests build cmd/lo0 themselves. Set LO0_E2E_BINARY to test a binary
// that was built elsewhere, such as a release build, instead.
package e2e
