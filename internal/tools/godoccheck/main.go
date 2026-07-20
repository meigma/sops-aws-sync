// Package main implements the repository's declaration documentation policy check.
package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// main checks each requested Go package tree and exits nonzero on policy violations.
func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run collects and reports all documentation-policy violations deterministically.
func run(roots []string) error {
	if len(roots) == 0 {
		return errors.New("at least one Go source root is required")
	}
	fileSet := token.NewFileSet()
	violations := make([]string, 0)
	packages := make(map[string]bool)
	for _, root := range roots {
		if err := inspectRoot(fileSet, root, packages, &violations); err != nil {
			return fmt.Errorf("walk Go source root %s: %w", root, err)
		}
	}
	for directory, documented := range packages {
		if !documented {
			violations = append(violations, directory+": package comment is missing")
		}
	}
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)

	return errors.New(strings.Join(violations, "\n"))
}

// inspectRoot parses every Go source file beneath one repository-owned root.
func inspectRoot(
	fileSet *token.FileSet,
	root string,
	packages map[string]bool,
	violations *[]string,
) error {
	//nolint:gosec // G703: roots are fixed repository task arguments, not untrusted input.
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		parsed, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		recordPackageComment(path, parsed, packages)
		*violations = append(*violations, inspectFile(fileSet, parsed)...)

		return nil
	})
}

// recordPackageComment records whether any file documents its package.
func recordPackageComment(path string, parsed *ast.File, packages map[string]bool) {
	directory := filepath.Dir(path)
	if parsed.Doc != nil && strings.HasPrefix(strings.TrimSpace(parsed.Doc.Text()), "Package "+parsed.Name.Name) {
		packages[directory] = true
	} else if _, seen := packages[directory]; !seen {
		packages[directory] = false
	}
}

// inspectFile checks named declarations and exported struct fields in one parsed file.
func inspectFile(fileSet *token.FileSet, file *ast.File) []string {
	violations := make([]string, 0)
	for _, declaration := range file.Decls {
		switch typed := declaration.(type) {
		case *ast.FuncDecl:
			if !startsWithName(typed.Doc, typed.Name.Name) {
				violations = append(violations, violation(fileSet, typed.Pos(), typed.Name.Name))
			}
		case *ast.GenDecl:
			violations = append(violations, inspectTypes(fileSet, typed)...)
		}
	}

	return violations
}

// inspectTypes checks named type declarations in one declaration group.
func inspectTypes(fileSet *token.FileSet, declaration *ast.GenDecl) []string {
	violations := make([]string, 0)
	for _, specification := range declaration.Specs {
		typeSpec, ok := specification.(*ast.TypeSpec)
		if !ok {
			continue
		}
		documentation := typeSpec.Doc
		if documentation == nil {
			documentation = declaration.Doc
		}
		if !startsWithName(documentation, typeSpec.Name.Name) {
			violations = append(violations, violation(fileSet, typeSpec.Pos(), typeSpec.Name.Name))
		}
		violations = append(violations, inspectExportedFields(fileSet, typeSpec)...)
	}

	return violations
}

// inspectExportedFields checks field comments on exported struct members.
func inspectExportedFields(fileSet *token.FileSet, specification *ast.TypeSpec) []string {
	structure, ok := specification.Type.(*ast.StructType)
	if !ok {
		return nil
	}
	violations := make([]string, 0)
	for _, field := range structure.Fields.List {
		for _, name := range field.Names {
			if name.IsExported() && !startsWithName(field.Doc, name.Name) {
				violations = append(violations, violation(fileSet, name.Pos(), name.Name))
			}
		}
	}

	return violations
}

// startsWithName reports whether a declaration comment begins with its identifier.
func startsWithName(group *ast.CommentGroup, name string) bool {
	return group != nil && strings.HasPrefix(strings.TrimSpace(group.Text()), name)
}

// violation formats one deterministic source-position failure.
func violation(fileSet *token.FileSet, position token.Pos, name string) string {
	location := fileSet.Position(position)

	return fmt.Sprintf("%s:%d: declaration %s needs a doc comment", location.Filename, location.Line, name)
}
