package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/meigma/sops-aws-sync/internal/application"
	"github.com/meigma/sops-aws-sync/internal/domain"
)

const sopsJSONSuffix = ".sops.json"

// Reader loads selected blobs from a local repository's committed object database.
type Reader struct {
	repositoryPath    string
	maxEncryptedBytes int64
}

// NewReader constructs a committed-source reader with a defensive blob limit.
func NewReader(repositoryPath string, maxEncryptedBytes int64) (*Reader, error) {
	if repositoryPath == "" {
		return nil, errors.New("repository path is empty")
	}
	if maxEncryptedBytes <= 0 {
		return nil, errors.New("maximum encrypted bytes must be positive")
	}

	return &Reader{repositoryPath: repositoryPath, maxEncryptedBytes: maxEncryptedBytes}, nil
}

// Load resolves one exact commit and returns matching regular blobs beneath the source root.
func (reader *Reader) Load(
	ctx context.Context,
	revision,
	sourceRoot string,
) (application.SourceSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return application.SourceSnapshot{}, fmt.Errorf("read committed source: %w", err)
	}
	repository, err := git.PlainOpen(reader.repositoryPath)
	if err != nil {
		return application.SourceSnapshot{}, fmt.Errorf("open repository: %w", err)
	}
	hash, err := repository.ResolveRevision(plumbing.Revision(revision))
	if err != nil {
		return application.SourceSnapshot{}, fmt.Errorf("resolve revision: %w", err)
	}
	commit, err := repository.CommitObject(*hash)
	if err != nil {
		return application.SourceSnapshot{}, fmt.Errorf("load commit object: %w", err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return application.SourceSnapshot{}, fmt.Errorf("load commit tree: %w", err)
	}
	selectedTree := tree
	if sourceRoot != "." {
		selectedTree, err = tree.Tree(sourceRoot)
		if err != nil {
			return application.SourceSnapshot{}, fmt.Errorf("load source root: %w", err)
		}
	}
	documents, err := reader.readDocuments(ctx, selectedTree, sourceRoot)
	if err != nil {
		return application.SourceSnapshot{}, err
	}
	exactRevision, err := domain.NewRevision(commit.Hash.String())
	if err != nil {
		return application.SourceSnapshot{}, fmt.Errorf("normalize revision: %w", err)
	}

	return application.SourceSnapshot{Revision: exactRevision, Documents: documents}, nil
}

// readDocuments reads matching regular blobs while polling cancellation between files.
func (reader *Reader) readDocuments(
	ctx context.Context,
	tree *object.Tree,
	sourceRoot string,
) ([]application.EncryptedDocument, error) {
	documents := make([]application.EncryptedDocument, 0, 1)
	walker := object.NewTreeWalker(tree, true, nil)
	defer walker.Close()
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("read committed source: %w", err)
		}
		name, entry, err := walker.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("walk committed source: %w", err)
		}
		if !strings.HasSuffix(name, sopsJSONSuffix) {
			continue
		}
		if entry.Mode != filemode.Regular {
			return nil, fmt.Errorf("selected source %q is not a regular Git blob", name)
		}
		document, err := reader.readDocument(tree, sourceRoot, name)
		if err != nil {
			return nil, err
		}
		documents = append(documents, document)
	}

	return documents, nil
}

// readDocument loads one already validated regular tree entry within its size limit.
func (reader *Reader) readDocument(
	tree *object.Tree,
	sourceRoot,
	name string,
) (application.EncryptedDocument, error) {
	file, err := tree.File(name)
	if err != nil {
		return application.EncryptedDocument{}, fmt.Errorf("load selected source %q: %w", name, err)
	}
	if file.Size > reader.maxEncryptedBytes {
		return application.EncryptedDocument{}, fmt.Errorf("selected source %q exceeds encrypted input limit", name)
	}
	blobReader, err := file.Reader()
	if err != nil {
		return application.EncryptedDocument{}, fmt.Errorf("open selected source %q: %w", name, err)
	}
	data, readErr := io.ReadAll(blobReader)
	closeErr := blobReader.Close()
	if readErr != nil {
		return application.EncryptedDocument{}, fmt.Errorf("read selected source %q: %w", name, readErr)
	}
	if closeErr != nil {
		return application.EncryptedDocument{}, fmt.Errorf("close selected source %q: %w", name, closeErr)
	}
	documentPath := name
	if sourceRoot != "." {
		documentPath = path.Join(sourceRoot, name)
	}

	return application.EncryptedDocument{Path: documentPath, Data: data}, nil
}
