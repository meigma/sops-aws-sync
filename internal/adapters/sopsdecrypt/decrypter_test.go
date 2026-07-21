package sopsdecrypt

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/getsops/sops/v3/cmd/sops/formats"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/sops-aws-sync/internal/domain"
)

// TestCanonicalizeJSONObject enforces strict object parsing and RFC 8785 output.
func TestCanonicalizeJSONObject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		plaintext string
		want      string
		wantError string
	}{
		{name: "canonicalizes nested object", plaintext: ` { "z": 1.0, "nested": { "b": true, "a": "value" } } `,
			want: `{"nested":{"a":"value","b":true},"z":1}`},
		{name: "rejects top-level array", plaintext: `[]`, wantError: "top-level JSON value must be an object"},
		{
			name:      "rejects duplicate nested member",
			plaintext: `{"nested":{"a":1,"a":2}}`,
			wantError: "duplicate object member",
		},
		{name: "rejects trailing data", plaintext: `{} {}`, wantError: "unexpected trailing JSON token"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := canonicalizeJSONObject([]byte(test.plaintext))
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, string(got))
		})
	}
}

// TestDecryptJSONVerifiesMACAndReturnsCanonicalValue proves the stable JSON binding path.
func TestDecryptJSONVerifiesMACAndReturnsCanonicalValue(t *testing.T) {
	t.Setenv("SOPS_AGE_KEY", "AGE-SECRET-KEY-"+"1G0Q5K9TV4REQ3ZSQRMTMG8NSWQGYT0T7TZ33RAZEE0GZYVZN0APSU24RK7")
	fixture, err := os.ReadFile("testdata/database.sops.json")
	require.NoError(t, err)
	decrypter, err := New(8 * 1024 * 1024)
	require.NoError(t, err)

	value, err := decrypter.Decrypt(context.Background(), domain.SourceFormatJSON, fixture)
	require.NoError(t, err)
	assert.JSONEq(t, `{"credential":"phase1-plaintext-sentinel","nested":{"a":"value","b":true},"z":1}`,
		string(value.CopyCanonicalJSON()))
}

// TestDecryptYAMLVerifiesMACAndMatchesJSON proves both encodings produce one canonical contract.
func TestDecryptYAMLVerifiesMACAndMatchesJSON(t *testing.T) {
	t.Setenv("SOPS_AGE_KEY", "AGE-SECRET-KEY-"+"1G0Q5K9TV4REQ3ZSQRMTMG8NSWQGYT0T7TZ33RAZEE0GZYVZN0APSU24RK7")
	fixture, err := os.ReadFile("testdata/database.sops.yaml")
	require.NoError(t, err)
	decrypter, err := New(8 * 1024 * 1024)
	require.NoError(t, err)

	value, err := decrypter.Decrypt(context.Background(), domain.SourceFormatYAML, fixture)
	require.NoError(t, err)
	assert.JSONEq(t,
		`{"credential":"phase1-plaintext-sentinel","nested":{"a":"value","b":true},"z":1}`,
		string(value.CopyCanonicalJSON()),
	)
}

// TestDecryptYAMLRejectsIntegrityMismatch proves plaintext-tree tampering cannot bypass the SOPS MAC.
func TestDecryptYAMLRejectsIntegrityMismatch(t *testing.T) {
	t.Setenv("SOPS_AGE_KEY", "AGE-SECRET-KEY-"+"1G0Q5K9TV4REQ3ZSQRMTMG8NSWQGYT0T7TZ33RAZEE0GZYVZN0APSU24RK7")
	fixture, err := os.ReadFile("testdata/database.sops.yaml")
	require.NoError(t, err)
	tampered := bytes.Replace(fixture, []byte("sops:\n"), []byte("marker_unencrypted: tampered\nsops:\n"), 1)
	require.NotEqual(t, fixture, tampered)
	decrypter, err := New(8 * 1024 * 1024)
	require.NoError(t, err)

	_, err = decrypter.Decrypt(context.Background(), domain.SourceFormatYAML, tampered)
	require.ErrorContains(t, err, "SOPS document decryption failed")
}

// TestDecryptRejectsYAMLOutsideTheJSONContract proves unsupported YAML fails closed.
func TestDecryptRejectsYAMLOutsideTheJSONContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		plaintext string
		wantError string
	}{
		{name: "multiple documents", plaintext: "a: 1\n---\nb: 2\n", wantError: "exactly one document"},
		{name: "sequence root", plaintext: "- a\n- b\n", wantError: "top-level YAML value must be a mapping"},
		{name: "duplicate key", plaintext: "a: 1\na: 2\n", wantError: "duplicate YAML mapping key"},
		{name: "non-string key", plaintext: "1: value\n", wantError: "YAML mapping keys must be strings"},
		{name: "timestamp", plaintext: "value: 2026-07-21\n", wantError: "non-JSON scalar"},
		{name: "alias", plaintext: "base: &base\n  a: 1\ncopy: *base\n", wantError: "unsupported value"},
		{name: "non-finite number", plaintext: "value: .inf\n", wantError: "non-JSON value"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			decrypter := &Decrypter{
				maxEncryptedBytes: 1024,
				decrypt: func(_ []byte, format formats.Format) ([]byte, error) {
					assert.Equal(t, formats.Yaml, format)

					return []byte(test.plaintext), nil
				},
			}

			_, err := decrypter.Decrypt(
				context.Background(),
				domain.SourceFormatYAML,
				[]byte("encrypted"),
			)
			require.ErrorContains(t, err, test.wantError)
			assert.NotContains(t, err.Error(), "value: 2026-07-21")
		})
	}
}

// TestDecryptDiscardsCanceledWork proves cancellation wins before a provider call begins.
func TestDecryptDiscardsCanceledWork(t *testing.T) {
	t.Parallel()

	decrypter, err := New(1024)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = decrypter.Decrypt(ctx, domain.SourceFormatJSON, []byte(`{}`))
	require.ErrorIs(t, err, context.Canceled)
}

// TestDecryptCancellationTerminatesTheWorkerProcess proves in-flight cancellation is terminal.
func TestDecryptCancellationTerminatesTheWorkerProcess(t *testing.T) {
	if os.Getenv("SOPS_AWS_SYNC_CANCELLATION_CHILD") == "1" {
		decrypter := &Decrypter{
			maxEncryptedBytes: 1024,
			decrypt: func(_ []byte, _ formats.Format) ([]byte, error) {
				select {}
			},
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		_, err := decrypter.Decrypt(ctx, domain.SourceFormatJSON, []byte(`{}`))
		require.ErrorIs(t, err, context.DeadlineExceeded)
		return
	}
	processContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	command := exec.CommandContext(
		processContext,
		os.Args[0],
		"-test.run=^TestDecryptCancellationTerminatesTheWorkerProcess$",
	)
	command.Env = append(os.Environ(), "SOPS_AWS_SYNC_CANCELLATION_CHILD=1")
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	require.NoError(t, processContext.Err())
}
