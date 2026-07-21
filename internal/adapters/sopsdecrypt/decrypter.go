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
	"go.yaml.in/yaml/v3"

	"github.com/meigma/sops-aws-sync/internal/domain"
)

const yamlMappingPairSize = 2

// Decrypter converts encrypted SOPS JSON or YAML bytes into canonical domain values.
type Decrypter struct {
	maxEncryptedBytes int
	decrypt           decryptFunction
}

// decryptFunction is the terminal non-context-aware SOPS binding seam.
type decryptFunction func(encrypted []byte, format formats.Format) ([]byte, error)

// New constructs a SOPS document decrypter with a defensive input limit.
func New(maxEncryptedBytes int) (*Decrypter, error) {
	if maxEncryptedBytes <= 0 {
		return nil, errors.New("maximum encrypted bytes must be positive")
	}

	return &Decrypter{maxEncryptedBytes: maxEncryptedBytes, decrypt: decryptSOPS}, nil
}

// decryptResult transports one terminal worker result without exposing plaintext.
type decryptResult struct {
	value domain.SecretValue
	err   error
}

// Decrypt verifies SOPS data and returns strict RFC 8785 canonical JSON.
func (decrypter *Decrypter) Decrypt(
	ctx context.Context,
	format domain.SourceFormat,
	encrypted []byte,
) (domain.SecretValue, error) {
	if err := ctx.Err(); err != nil {
		return domain.SecretValue{}, fmt.Errorf("decrypt SOPS document: %w", err)
	}
	if len(encrypted) > decrypter.maxEncryptedBytes {
		return domain.SecretValue{}, errors.New("encrypted SOPS document exceeds input limit")
	}
	sopsFormat, ok := sopsFormatForSource(format)
	if !ok {
		return domain.SecretValue{}, errors.New("unsupported SOPS source format")
	}
	result := make(chan decryptResult, 1)
	isolated := append([]byte(nil), encrypted...)
	go func() {
		result <- decrypter.decryptAndCanonicalize(isolated, format, sopsFormat)
	}()

	select {
	case <-ctx.Done():
		return domain.SecretValue{}, fmt.Errorf("decrypt SOPS document: %w", ctx.Err())
	case completed := <-result:
		if err := ctx.Err(); err != nil {
			return domain.SecretValue{}, fmt.Errorf("discard canceled SOPS result: %w", err)
		}
		return completed.value, completed.err
	}
}

// sopsFormatForSource maps the closed domain format set to SOPS's stable API.
func sopsFormatForSource(format domain.SourceFormat) (formats.Format, bool) {
	switch format {
	case domain.SourceFormatJSON:
		return formats.Json, true
	case domain.SourceFormatYAML:
		return formats.Yaml, true
	default:
		return formats.Binary, false
	}
}

// decryptSOPS invokes the stable non-context-aware SOPS binding.
func decryptSOPS(encrypted []byte, format formats.Format) ([]byte, error) {
	return decrypt.DataWithFormat(encrypted, format)
}

// decryptAndCanonicalize performs the terminal decrypt and strict conversion.
func (decrypter *Decrypter) decryptAndCanonicalize(
	encrypted []byte,
	sourceFormat domain.SourceFormat,
	sopsFormat formats.Format,
) decryptResult {
	plaintext, err := decrypter.decrypt(encrypted, sopsFormat)
	if err != nil {
		return decryptResult{err: errors.New("SOPS document decryption failed")}
	}
	if sourceFormat == domain.SourceFormatYAML {
		plaintext, err = convertYAMLToJSON(plaintext)
		if err != nil {
			return decryptResult{err: fmt.Errorf("validate decrypted YAML: %w", err)}
		}
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

// convertYAMLToJSON accepts one JSON-compatible mapping and emits JSON bytes.
func convertYAMLToJSON(plaintext []byte) ([]byte, error) {
	if !utf8.Valid(plaintext) {
		return nil, errors.New("plaintext is not valid UTF-8")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(plaintext))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, errors.New("YAML plaintext is invalid")
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("YAML plaintext must contain exactly one document")
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("top-level YAML value must be a mapping")
	}
	value, err := yamlNodeToJSONValue(document.Content[0])
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("YAML plaintext contains a non-JSON value")
	}

	return encoded, nil
}

// yamlNodeToJSONValue converts the supported YAML node subset without losing type intent.
func yamlNodeToJSONValue(node *yaml.Node) (any, error) {
	switch node.Kind {
	case yaml.MappingNode:
		return yamlMappingToJSONValue(node)
	case yaml.SequenceNode:
		values := make([]any, 0, len(node.Content))
		for _, child := range node.Content {
			value, err := yamlNodeToJSONValue(child)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}

		return values, nil
	case yaml.ScalarNode:
		return yamlScalarToJSONValue(node)
	case yaml.DocumentNode, yaml.AliasNode:
		return nil, errors.New("YAML plaintext contains an unsupported value")
	default:
		return nil, errors.New("YAML plaintext contains an unsupported value")
	}
}

// yamlMappingToJSONValue enforces string, unique object member names.
func yamlMappingToJSONValue(node *yaml.Node) (map[string]any, error) {
	values := make(map[string]any, len(node.Content)/yamlMappingPairSize)
	for index := 0; index < len(node.Content); index += yamlMappingPairSize {
		key := node.Content[index]
		if key.Kind != yaml.ScalarNode || key.ShortTag() != "!!str" {
			return nil, errors.New("YAML mapping keys must be strings")
		}
		if _, duplicate := values[key.Value]; duplicate {
			return nil, errors.New("duplicate YAML mapping key")
		}
		value, err := yamlNodeToJSONValue(node.Content[index+1])
		if err != nil {
			return nil, err
		}
		values[key.Value] = value
	}

	return values, nil
}

// yamlScalarToJSONValue converts only scalar types present in JSON's data model.
func yamlScalarToJSONValue(node *yaml.Node) (any, error) {
	switch node.ShortTag() {
	case "!!null":
		return json.RawMessage("null"), nil
	case "!!str":
		return node.Value, nil
	case "!!bool":
		var value bool
		if err := node.Decode(&value); err != nil {
			return nil, errors.New("YAML boolean is invalid")
		}

		return value, nil
	case "!!int":
		var value any
		if err := node.Decode(&value); err != nil {
			return nil, errors.New("YAML integer is invalid")
		}
		switch value := value.(type) {
		case int, int64, uint64:
			return value, nil
		default:
			return nil, errors.New("YAML integer is outside the JSON profile")
		}
	case "!!float":
		var value float64
		if err := node.Decode(&value); err != nil {
			return nil, errors.New("YAML number is invalid")
		}

		return value, nil
	default:
		return nil, errors.New("YAML plaintext contains a non-JSON scalar")
	}
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
