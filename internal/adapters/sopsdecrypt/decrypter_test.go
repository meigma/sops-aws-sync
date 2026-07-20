package sopsdecrypt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalizeJSONObject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		plaintext string
		want      string
		wantError string
	}{
		{
			name:      "canonicalizes nested object",
			plaintext: ` { "z": 1.0, "nested": { "b": true, "a": "value" } } `,
			want:      `{"nested":{"a":"value","b":true},"z":1}`,
		},
		{
			name:      "rejects top-level array",
			plaintext: `[]`,
			wantError: "top-level JSON value must be an object",
		},
		{
			name:      "rejects duplicate nested member",
			plaintext: `{"nested":{"a":1,"a":2}}`,
			wantError: `duplicate object member "a"`,
		},
		{
			name:      "rejects trailing data",
			plaintext: `{} {}`,
			wantError: "unexpected trailing JSON token",
		},
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
