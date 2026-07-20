package application_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhaseOneProductionCodeDoesNotImportAWSClient(t *testing.T) {
	t.Parallel()

	forbiddenPrefix := "github.com/aws/aws-sdk-go"
	err := filepath.WalkDir("..", func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), filePath, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			assert.Falsef(
				t,
				strings.HasPrefix(importPath, forbiddenPrefix),
				"production file %s imports AWS package %s",
				filePath,
				importPath,
			)
		}

		return nil
	})
	require.NoError(t, err)
}
