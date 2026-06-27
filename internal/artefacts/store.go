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

func (s LocalStore) RegisterFile(ctx context.Context, inputPath string, meta RegisterMeta) (model.SourceArtefact, error) {
	if err := validateLocalInput(inputPath, meta.AllowedRootDirs); err != nil {
		return model.SourceArtefact{}, err
	}
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
	file, err := os.Open(inputPath)
	if err != nil {
		return model.SourceArtefact{}, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return model.SourceArtefact{}, err
	}
	sum := hex.EncodeToString(hash.Sum(nil))
	artefactID := fmt.Sprintf("artefact:%s", sum)
	extension := filepath.Ext(inputPath)
	if extension == "" {
		extension = ".bin"
	}
	targetDir := filepath.Join(root, sum[:2])
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return model.SourceArtefact{}, err
	}
	target := filepath.Join(targetDir, sum+extension)
	if err := copyRegularFile(inputPath, target); err != nil {
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

func validateLocalInput(inputPath string, allowedRootDirs []string) error {
	if inputPath == "" {
		return errors.New("artefact path is required")
	}
	if parsed, err := url.Parse(inputPath); err == nil && parsed.Scheme != "" && parsed.Scheme != "file" && filepath.VolumeName(inputPath) == "" {
		return fmt.Errorf("artefact path must be local, got %q", parsed.Scheme)
	}
	status, err := os.Lstat(inputPath)
	if err != nil {
		return err
	}
	if status.Mode()&os.ModeSymlink != 0 {
		return errors.New("artefact path must not be a symbolic link")
	}
	if !status.Mode().IsRegular() {
		return errors.New("artefact path must be a regular file")
	}
	if len(allowedRootDirs) == 0 {
		return nil
	}
	resolvedInput, err := filepath.Abs(inputPath)
	if err != nil {
		return err
	}
	for _, root := range allowedRootDirs {
		resolvedRoot, err := filepath.Abs(root)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(resolvedRoot, resolvedInput)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return nil
		}
	}
	return errors.New("artefact path is outside the allowed roots")
}

func copyRegularFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
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
