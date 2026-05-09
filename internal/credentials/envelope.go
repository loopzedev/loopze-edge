// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package credentials

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// envelopeVersion is the current credentials.json schema version. The
// envelope wraps two independent maps (node credentials and certificate
// entries) so they share one encrypted file and one master key.
const envelopeVersion = 2

// ErrEnvelopeUnsupported is returned when a credentials file is encountered
// whose declared version is newer than envelopeVersion. Older versions are
// auto-upgraded on read instead of erroring.
var ErrEnvelopeUnsupported = errors.New("credentials: envelope version is newer than this binary supports")

// envelope is the on-disk shape of credentials.json (after decryption).
// The credentials half is held as raw JSON so this package does not need
// to know the node-credential schema; consumers unmarshal it themselves.
type envelope struct {
	Version     int                        `json:"version"`
	Credentials map[string]json.RawMessage `json:"credentials"`
	Certs       map[string]CertEntry       `json:"certs"`
}

// newEnvelope returns a fresh envelope at the current version with empty
// maps. Used for first-run state and as the default for tests.
func newEnvelope() *envelope {
	return &envelope{
		Version:     envelopeVersion,
		Credentials: map[string]json.RawMessage{},
		Certs:       map[string]CertEntry{},
	}
}

// decodeEnvelope parses an in-memory credentials document. It accepts:
//
//   - nil / empty input → fresh empty envelope (first-run state)
//   - v2 documents with a top-level "version" field → decoded as-is
//   - legacy v1 documents (a bare map of node-id → credential blob) →
//     wrapped into a v2 envelope with empty Certs
//
// A document whose declared version is greater than envelopeVersion is
// rejected with ErrEnvelopeUnsupported so an older binary cannot silently
// drop fields written by a newer one.
func decodeEnvelope(data []byte) (*envelope, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return newEnvelope(), nil
	}

	// Peek for the "version" field without committing to a full parse.
	var probe struct {
		Version *int `json:"version"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("credentials: failed to parse envelope: %w", err)
	}

	if probe.Version == nil {
		// Legacy v1: the entire document is the credentials map.
		legacy := map[string]json.RawMessage{}
		if err := json.Unmarshal(data, &legacy); err != nil {
			return nil, fmt.Errorf("credentials: failed to parse legacy v1 credentials: %w", err)
		}
		env := newEnvelope()
		env.Credentials = legacy
		return env, nil
	}

	if *probe.Version > envelopeVersion {
		return nil, fmt.Errorf("%w: file is v%d, binary supports up to v%d", ErrEnvelopeUnsupported, *probe.Version, envelopeVersion)
	}

	env := newEnvelope()
	if err := json.Unmarshal(data, env); err != nil {
		return nil, fmt.Errorf("credentials: failed to parse v%d envelope: %w", *probe.Version, err)
	}

	// Defensive: a v2 file with null sub-maps must not panic later.
	if env.Credentials == nil {
		env.Credentials = map[string]json.RawMessage{}
	}
	if env.Certs == nil {
		env.Certs = map[string]CertEntry{}
	}
	env.Version = envelopeVersion
	return env, nil
}

// encodeEnvelope serialises an envelope at the current version. nil input
// is treated as an empty envelope so callers do not need a special-case
// branch for first-write state.
func encodeEnvelope(env *envelope) ([]byte, error) {
	if env == nil {
		env = newEnvelope()
	}
	out := envelope{
		Version:     envelopeVersion,
		Credentials: env.Credentials,
		Certs:       env.Certs,
	}
	if out.Credentials == nil {
		out.Credentials = map[string]json.RawMessage{}
	}
	if out.Certs == nil {
		out.Certs = map[string]CertEntry{}
	}
	data, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("credentials: failed to encode envelope: %w", err)
	}
	return data, nil
}
