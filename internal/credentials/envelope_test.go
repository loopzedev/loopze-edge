// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package credentials

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestDecodeEnvelope_EmptyInput(t *testing.T) {
	cases := [][]byte{nil, {}, []byte("   \n\t")}
	for _, in := range cases {
		env, err := decodeEnvelope(in)
		if err != nil {
			t.Fatalf("empty input should not error, got %v", err)
		}
		if env.Version != envelopeVersion {
			t.Errorf("Version = %d, want %d", env.Version, envelopeVersion)
		}
		if len(env.Credentials) != 0 {
			t.Errorf("Credentials should be empty, got %d entries", len(env.Credentials))
		}
		if len(env.Certs) != 0 {
			t.Errorf("Certs should be empty, got %d entries", len(env.Certs))
		}
	}
}

func TestDecodeEnvelope_LegacyV1(t *testing.T) {
	// A legacy v1 file is a bare map of node-id → credential blob with
	// no top-level "version" field.
	legacy := []byte(`{"node-abc":{"password":"secret"},"node-def":{"token":"xyz"}}`)
	env, err := decodeEnvelope(legacy)
	if err != nil {
		t.Fatalf("decode legacy: %v", err)
	}
	if env.Version != envelopeVersion {
		t.Errorf("Version = %d, want %d (auto-upgraded)", env.Version, envelopeVersion)
	}
	if len(env.Credentials) != 2 {
		t.Errorf("Credentials count = %d, want 2", len(env.Credentials))
	}
	if _, ok := env.Credentials["node-abc"]; !ok {
		t.Error("expected node-abc in Credentials")
	}
	if len(env.Certs) != 0 {
		t.Errorf("Certs should be empty for legacy v1, got %d", len(env.Certs))
	}
}

func TestDecodeEnvelope_V2RoundtripPreservesBothMaps(t *testing.T) {
	original := newEnvelope()
	original.Credentials["node-1"] = json.RawMessage(`{"password":"hunter2"}`)
	original.Certs["ca-root"] = CertEntry{
		ID:        "ca-root",
		Name:      "Internal Root",
		Type:      TypeCABundle,
		Source:    SourceInline,
		CertPEM:   "-----BEGIN CERTIFICATE-----\nMII...\n-----END CERTIFICATE-----",
		Subject:   "CN=internal-root",
		NotAfter:  time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
		CreatedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
	}

	data, err := encodeEnvelope(original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeEnvelope(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded.Credentials) != 1 {
		t.Errorf("Credentials count = %d, want 1", len(decoded.Credentials))
	}
	if string(decoded.Credentials["node-1"]) != `{"password":"hunter2"}` {
		t.Errorf("Credentials node-1 = %s", decoded.Credentials["node-1"])
	}
	got, ok := decoded.Certs["ca-root"]
	if !ok {
		t.Fatal("ca-root cert missing after roundtrip")
	}
	if got.Subject != "CN=internal-root" {
		t.Errorf("Subject = %q, want CN=internal-root", got.Subject)
	}
	if !got.NotAfter.Equal(original.Certs["ca-root"].NotAfter) {
		t.Errorf("NotAfter = %v, want %v", got.NotAfter, original.Certs["ca-root"].NotAfter)
	}
	if got.CertPEM != original.Certs["ca-root"].CertPEM {
		t.Error("CertPEM differs after roundtrip")
	}
}

func TestDecodeEnvelope_V2WithNullSubMaps(t *testing.T) {
	// A v2 file with explicit null for certs / credentials must decode to
	// empty maps, not leave nil that consumers would have to nil-check.
	data := []byte(`{"version":2,"credentials":null,"certs":null}`)
	env, err := decodeEnvelope(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Credentials == nil {
		t.Error("Credentials should be non-nil empty map")
	}
	if env.Certs == nil {
		t.Error("Certs should be non-nil empty map")
	}
}

func TestDecodeEnvelope_FutureVersionRejected(t *testing.T) {
	data := []byte(`{"version":99,"credentials":{},"certs":{}}`)
	_, err := decodeEnvelope(data)
	if !errors.Is(err, ErrEnvelopeUnsupported) {
		t.Errorf("got %v, want ErrEnvelopeUnsupported", err)
	}
}

func TestDecodeEnvelope_MalformedJSON(t *testing.T) {
	data := []byte(`{not-json}`)
	if _, err := decodeEnvelope(data); err == nil {
		t.Error("expected error on malformed JSON")
	}
}

func TestEncodeEnvelope_NilInput(t *testing.T) {
	data, err := encodeEnvelope(nil)
	if err != nil {
		t.Fatalf("encode nil: %v", err)
	}
	env, err := decodeEnvelope(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Version != envelopeVersion {
		t.Errorf("Version = %d, want %d", env.Version, envelopeVersion)
	}
	if len(env.Credentials) != 0 || len(env.Certs) != 0 {
		t.Error("nil input should encode to empty maps")
	}
}

func TestEncodeEnvelope_AlwaysWritesCurrentVersion(t *testing.T) {
	// Even if a caller hands us an envelope claiming v1, the encoded form
	// must declare envelopeVersion. Otherwise round-tripping a legacy file
	// would write back v1 and never upgrade on disk.
	env := &envelope{
		Version:     1,
		Credentials: map[string]json.RawMessage{"a": json.RawMessage(`{}`)},
	}
	data, err := encodeEnvelope(env)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeEnvelope(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Version != envelopeVersion {
		t.Errorf("Version = %d, want %d (encode must always upgrade)", decoded.Version, envelopeVersion)
	}
}
