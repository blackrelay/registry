package publisher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type LocalStore struct {
	Root string
}

func (s LocalStore) PutObject(ctx context.Context, object Object) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	root := strings.TrimSpace(s.Root)
	if root == "" {
		return errors.New("local publish root is required")
	}
	target, err := safeLocalObjectPath(root, object.Key)
	if err != nil {
		return err
	}
	if !object.AllowOverwrite {
		if _, err := os.Stat(target); err == nil {
			return fmt.Errorf("object %s already exists", object.Key)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(target), ".publish-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tempName)
		}
	}()
	if object.SourcePath != "" {
		data, err := os.ReadFile(object.SourcePath)
		if err != nil {
			_ = temp.Close()
			return err
		}
		if _, err := temp.Write(data); err != nil {
			_ = temp.Close()
			return err
		}
	} else {
		if _, err := temp.Write(object.Body); err != nil {
			_ = temp.Close()
			return err
		}
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if object.AllowOverwrite {
		_ = os.Remove(target)
	}
	if err := os.Rename(tempName, target); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func safeLocalObjectPath(root, key string) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", errors.New("object key is required")
	}
	if filepath.IsAbs(key) {
		return "", fmt.Errorf("unsafe object key %q: absolute paths are not allowed", key)
	}
	cleanKey := filepath.Clean(filepath.FromSlash(strings.ReplaceAll(key, "\\", "/")))
	if cleanKey == "." || cleanKey == ".." || strings.HasPrefix(cleanKey, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("unsafe object key %q: parent traversal is not allowed", key)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	targetAbs, err := filepath.Abs(filepath.Join(rootAbs, cleanKey))
	if err != nil {
		return "", err
	}
	if targetAbs != rootAbs && !strings.HasPrefix(targetAbs, rootAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("unsafe object key %q: target escapes publish root", key)
	}
	return targetAbs, nil
}
