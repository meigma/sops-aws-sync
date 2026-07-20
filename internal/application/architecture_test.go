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

// TestDomainUsesOnlyTheStandardLibrary enforces the pure business-logic boundary.
func TestDomainUsesOnlyTheStandardLibrary(t *testing.T) {
	t.Parallel()

	imports := productionImports(t, "../domain")
	for sourceFile, paths := range imports {
		for _, importPath := range paths {
			assert.NotContainsf(
				t,
				importPath,
				".",
				"domain file %s imports non-standard package %s",
				sourceFile,
				importPath,
			)
		}
	}
}

// TestApplicationImportsOnlyDomainAndStandardLibrary enforces ports owned by the consumer.
func TestApplicationImportsOnlyDomainAndStandardLibrary(t *testing.T) {
	t.Parallel()

	imports := productionImports(t, ".")
	for sourceFile, paths := range imports {
		for _, importPath := range paths {
			allowed := !strings.Contains(importPath, ".") ||
				importPath == "github.com/meigma/sops-aws-sync/internal/domain"
			assert.Truef(t, allowed, "application file %s imports forbidden package %s", sourceFile, importPath)
		}
	}
}

// productionImports returns imports from non-test Go files beneath one package directory.
func productionImports(t *testing.T, root string) map[string][]string {
	t.Helper()
	imports := make(map[string][]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			imports[path] = append(imports[path], importPath)
		}

		return nil
	})
	require.NoError(t, err)

	return imports
}
