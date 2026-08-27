`wire.json` is a synthetic serialization fixture, not an evaluator golden. Its
request and result cover every field in the Go contract, including optional
fields and all overlay reports; minimal variants pin omission and empty arrays.

`wire_test.go` checks strict decoding, lossless JSON round trips, field coverage,
verdict constants and numeric-ID keys. The web `contract.test.ts` compiles this
same JSON as a literal against `EngineRequest` and `EngineResult`, without casts.
Go zero-value null serialization is pinned separately in `TestWireEmptyAndNull`.

Update this fixture deliberately when changing the wire contract. Do not change
it merely to make a behavior-preserving refactor pass. No generator is required.
