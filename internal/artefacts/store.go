package artefacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blackrelay/registry/internal/model"
)

const ImporterVersion = "0.1.0"

type Store interface {
	RegisterFile(ctx context.Context, inputPath string, meta RegisterMeta) (model.SourceArtefact, error)
}

type RegisterMeta struct {
	SourceID        string
	SourceKind      model.SourceKind
	Kind            string
	ArtefactKind    string
	Environment     model.Environment
	ContentType     string
	RowCount        int64
	ImporterName    string
	ClientBuild     string
	PatchLabel      string
	Cycle           *int
	ReviewStatus    model.ReviewStatus
	Notes           string
	AllowedRootDirs []string
}

type LocalStore struct {
	Root string
	Now  func() time.Time
}

type LocalInput struct {
	Path      string
	Extension string
}

func (s LocalStore) RegisterFile(ctx context.Context, inputPath string, meta RegisterMeta) (model.SourceArtefact, error) {
	file, input, err := OpenLocalInput(inputPath, meta.AllowedRootDirs)
	if err != nil {
		return model.SourceArtefact{}, err
	}
	defer file.Close()
	if meta.SourceID == "" || meta.Kind == "" || meta.ImporterName == "" {
		return model.SourceArtefact{}, errors.New("source id, artefact kind and importer name are required")
	}
	sourceKind := meta.SourceKind
	if sourceKind == "" {
		sourceKind = model.SourceKindStaticClientData
	}
	artefactKind := meta.ArtefactKind
	if artefactKind == "" {
		artefactKind = meta.Kind
	}
	root := s.Root
	if root == "" {
		root = "artefacts"
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return model.SourceArtefact{}, err
	}
	sum := hex.EncodeToString(hash.Sum(nil))
	artefactID := fmt.Sprintf("artefact:%s", sum)
	targetDir := filepath.Join(root, sum[:2])
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return model.SourceArtefact{}, err
	}
	target := filepath.Join(targetDir, sum+input.Extension)
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return model.SourceArtefact{}, err
	}
	if err := copyOpenFile(file, target); err != nil {
		return model.SourceArtefact{}, err
	}
	return model.SourceArtefact{
		ID:              artefactID,
		SourceID:        meta.SourceID,
		SourceKind:      sourceKind,
		Kind:            meta.Kind,
		ArtefactKind:    artefactKind,
		Environment:     meta.Environment,
		PathOrURI:       target,
		SHA256:          sum,
		SizeBytes:       size,
		RowCount:        meta.RowCount,
		ContentType:     meta.ContentType,
		ExtractedAt:     now,
		ImporterName:    meta.ImporterName,
		ImporterVersion: ImporterVersion,
		ClientBuild:     meta.ClientBuild,
		PatchLabel:      meta.PatchLabel,
		Cycle:           meta.Cycle,
		ReviewStatus:    meta.ReviewStatus,
		Notes:           meta.Notes,
		CreatedAt:       now,
	}, nil
}

func ReadLocalInput(inputPath string, allowedRootDirs []string) ([]byte, LocalInput, error) {
	rootedFile, input, err := OpenLocalInput(inputPath, allowedRootDirs)
	if err != nil {
		return nil, LocalInput{}, err
	}
	defer rootedFile.Close()
	data, err := io.ReadAll(rootedFile)
	if err != nil {
		return nil, LocalInput{}, err
	}
	return data, input, nil
}

func OpenLocalInput(inputPath string, allowedRootDirs []string) (*os.File, LocalInput, error) {
	if inputPath == "" {
		return nil, LocalInput{}, errors.New("artefact path is required")
	}
	if parsed, err := url.Parse(inputPath); err == nil && parsed.Scheme != "" && parsed.Scheme != "file" && filepath.VolumeName(inputPath) == "" {
		return nil, LocalInput{}, fmt.Errorf("artefact path must be local, got %q", parsed.Scheme)
	}
	resolvedInput, err := filepath.Abs(inputPath)
	if err != nil {
		return nil, LocalInput{}, err
	}
	if len(allowedRootDirs) == 0 {
		return nil, LocalInput{}, errors.New("at least one allowed artefact root is required")
	}
	for _, root := range allowedRootDirs {
		resolvedRoot, err := filepath.Abs(root)
		if err != nil {
			return nil, LocalInput{}, err
		}
		rel, err := filepath.Rel(resolvedRoot, resolvedInput)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			continue
		}
		rootDir, err := os.OpenRoot(resolvedRoot)
		if err != nil {
			return nil, LocalInput{}, err
		}
		defer rootDir.Close()
		status, err := rootDir.Lstat(rel)
		if err != nil {
			return nil, LocalInput{}, err
		}
		if status.Mode()&os.ModeSymlink != 0 {
			return nil, LocalInput{}, errors.New("artefact path must not be a symbolic link")
		}
		if !status.Mode().IsRegular() {
			return nil, LocalInput{}, errors.New("artefact path must be a regular file")
		}
		file, err := rootDir.Open(rel)
		if err != nil {
			return nil, LocalInput{}, err
		}
		return file, LocalInput{
			Path:      resolvedInput,
			Extension: safeArtefactExtension(resolvedInput),
		}, nil
	}
	return nil, LocalInput{}, errors.New("artefact path is outside the allowed roots")
}

func safeArtefactExtension(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".bin", ".fsdbinary", ".json", ".jsonl", ".txt", ".yaml", ".yml":
		return strings.ToLower(filepath.Ext(path))
	default:
		return ".bin"
	}
}

func copyOpenFile(input *os.File, target string) error {
	output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}
