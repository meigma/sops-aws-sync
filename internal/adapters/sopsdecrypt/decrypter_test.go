package sopsdecrypt

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestDecryptJSONVerifiesMACAndReturnsCanonicalValue proves the stable SOPS binding path.
func TestDecryptJSONVerifiesMACAndReturnsCanonicalValue(t *testing.T) {
	t.Setenv("SOPS_AGE_KEY", "AGE-SECRET-KEY-"+"1G0Q5K9TV4REQ3ZSQRMTMG8NSWQGYT0T7TZ33RAZEE0GZYVZN0APSU24RK7")
	fixture, err := os.ReadFile("testdata/database.sops.json")
	require.NoError(t, err)
	decrypter, err := New(8 * 1024 * 1024)
	require.NoError(t, err)

	value, err := decrypter.DecryptJSON(context.Background(), fixture)
	require.NoError(t, err)
	assert.JSONEq(t, `{"credential":"phase1-plaintext-sentinel","nested":{"a":"value","b":true},"z":1}`,
		string(value.CopyCanonicalJSON()))
}

// TestDecryptJSONDiscardsCanceledWork proves cancellation wins before a provider call begins.
func TestDecryptJSONDiscardsCanceledWork(t *testing.T) {
	t.Parallel()

	decrypter, err := New(1024)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = decrypter.DecryptJSON(ctx, []byte(`{}`))
	require.ErrorIs(t, err, context.Canceled)
}
