package main

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"errors"
	"io"
)

// maxInflatedBytes mirrors the client's cap on a decoded scenario
// (web/src/app/shareCodec.ts MAX_INFLATED_BYTES); a token that expands past it
// would be rejected on load anyway.
const maxInflatedBytes = 4 << 20

// zeroDict stands in for the client's preset dictionary (derived from the web
// app's templates, so the server can't and shouldn't carry it). Back-references
// into the dictionary then resolve to zeros instead of failing, which is enough
// to check that the stream is a well-formed, terminated deflate stream.
var zeroDict = make([]byte, 32<<10)

var errNotScenario = errors.New("token is not a share payload")

// looksLikeScenario reports whether t is base64url wrapping one complete raw
// deflate stream that inflates to a bounded size. It doesn't (and can't, without
// the dictionary) read the scenario, so a crafted stream still passes; what it
// rejects is the cheap junk — random or hand-typed base64url — that would
// otherwise cost as much storage as a real share.
func looksLikeScenario(t string) error {
	raw, err := base64.RawURLEncoding.DecodeString(t)
	if err != nil {
		return errNotScenario
	}
	src := bytes.NewReader(raw)
	zr := flate.NewReaderDict(src, zeroDict)
	defer zr.Close()
	n, err := io.Copy(io.Discard, io.LimitReader(zr, maxInflatedBytes+1))
	if err != nil || n == 0 || n > maxInflatedBytes || src.Len() != 0 {
		return errNotScenario
	}
	return nil
}
