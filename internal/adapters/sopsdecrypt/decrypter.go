package sopsdecrypt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/getsops/sops/v3/cmd/sops/formats"
	"github.com/getsops/sops/v3/decrypt"
	"github.com/gowebpki/jcs"

	"github.com/meigma/sops-aws-sync/internal/domain"
)

// Decrypter converts encrypted SOPS JSON bytes into canonical domain values.
type Decrypter struct {
	maxEncryptedBytes int
	decrypt           decryptFunction
}

// decryptFunction is the terminal non-context-aware SOPS binding seam.
type decryptFunction func(encrypted []byte) ([]byte, error)

// New constructs a SOPS JSON decrypter with a defensive input limit.
func New(maxEncryptedBytes int) (*Decrypter, error) {
	if maxEncryptedBytes <= 0 {
		return nil, errors.New("maximum encrypted bytes must be positive")
	}

	return &Decrypter{maxEncryptedBytes: maxEncryptedBytes, decrypt: decryptSOPSJSON}, nil
}

// decryptResult transports one terminal worker result without exposing plaintext.
type decryptResult struct {
	value domain.SecretValue
	err   error
}

// DecryptJSON verifies SOPS data and returns strict RFC 8785 canonical JSON.
func (decrypter *Decrypter) DecryptJSON(ctx context.Context, encrypted []byte) (domain.SecretValue, error) {
	if err := ctx.Err(); err != nil {
		return domain.SecretValue{}, fmt.Errorf("decrypt SOPS JSON: %w", err)
	}
	if len(encrypted) > decrypter.maxEncryptedBytes {
		return domain.SecretValue{}, errors.New("encrypted SOPS JSON exceeds input limit")
	}
	result := make(chan decryptResult, 1)
	isolated := append([]byte(nil), encrypted...)
	go func() {
		result <- decrypter.decryptAndCanonicalize(isolated)
	}()

	select {
	case <-ctx.Done():
		return domain.SecretValue{}, fmt.Errorf("decrypt SOPS JSON: %w", ctx.Err())
	case completed := <-result:
		if err := ctx.Err(); err != nil {
			return domain.SecretValue{}, fmt.Errorf("discard canceled SOPS result: %w", err)
		}
		return completed.value, completed.err
	}
}

// decryptSOPSJSON invokes the stable non-context-aware SOPS binding.
func decryptSOPSJSON(encrypted []byte) ([]byte, error) {
	return decrypt.DataWithFormat(encrypted, formats.Json)
}

// decryptAndCanonicalize performs the terminal decrypt and strict JSON conversion.
func (decrypter *Decrypter) decryptAndCanonicalize(encrypted []byte) decryptResult {
	plaintext, err := decrypter.decrypt(encrypted)
	if err != nil {
		return decryptResult{err: errors.New("SOPS JSON decryption failed")}
	}
	canonical, err := canonicalizeJSONObject(plaintext)
	if err != nil {
		return decryptResult{err: fmt.Errorf("validate decrypted JSON: %w", err)}
	}
	value, err := domain.NewSecretValue(canonical)
	if err != nil {
		return decryptResult{err: fmt.Errorf("construct canonical secret value: %w", err)}
	}

	return decryptResult{value: value}
}

// canonicalizeJSONObject enforces the strict plaintext contract before JCS transformation.
func canonicalizeJSONObject(plaintext []byte) ([]byte, error) {
	if !utf8.Valid(plaintext) {
		return nil, errors.New("plaintext is not valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	decoder.UseNumber()
	opening, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("read top-level JSON value: %w", err)
	}
	delimiter, ok := opening.(json.Delim)
	if !ok || delimiter != '{' {
		return nil, errors.New("top-level JSON value must be an object")
	}
	if consumeErr := consumeObject(decoder); consumeErr != nil {
		return nil, consumeErr
	}
	if token, trailingErr := decoder.Token(); trailingErr != io.EOF {
		if trailingErr != nil {
			return nil, fmt.Errorf("read trailing JSON data: %w", trailingErr)
		}
		return nil, fmt.Errorf("unexpected trailing JSON token %v", token)
	}
	canonical, err := jcs.Transform(plaintext)
	if err != nil {
		return nil, fmt.Errorf("apply RFC 8785 canonicalization: %w", err)
	}

	return canonical, nil
}

// consumeObject validates one JSON object and rejects duplicate member names.
func consumeObject(decoder *json.Decoder) error {
	seen := make(map[string]struct{})
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("read object member: %w", err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return errors.New("object member name is not a string")
		}
		if _, duplicate := seen[key]; duplicate {
			return errors.New("duplicate object member")
		}
		seen[key] = struct{}{}
		if err := consumeValue(decoder); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("close JSON object: %w", err)
	}
	if closing != json.Delim('}') {
		return errors.New("JSON object has an invalid closing delimiter")
	}

	return nil
}

// consumeArray validates each value in one JSON array.
func consumeArray(decoder *json.Decoder) error {
	for decoder.More() {
		if err := consumeValue(decoder); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("close JSON array: %w", err)
	}
	if closing != json.Delim(']') {
		return errors.New("JSON array has an invalid closing delimiter")
	}

	return nil
}

// consumeValue validates one scalar or recursively validates one compound value.
func consumeValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("read JSON value: %w", err)
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		return consumeObject(decoder)
	case '[':
		return consumeArray(decoder)
	default:
		return fmt.Errorf("unexpected opening delimiter %q", delimiter)
	}
}
