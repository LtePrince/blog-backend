// Package content reads Markdown and assets in the content repository.
package content

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
)

func validPath(name string) bool {
	if name == "." || !fs.ValidPath(name) || strings.ContainsAny(name, "\\\x00") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") {
			return false
		}
	}
	return true
}

// Open a regular file beneath the repository, excluding hidden paths and links.
func Open(root, name string) (*os.File, error) {
	if !validPath(name) {
		return nil, fs.ErrInvalid
	}
	dir, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	prefix := ""
	for _, part := range strings.Split(name, "/") {
		prefix = path.Join(prefix, part)
		info, err := dir.Lstat(prefix)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || (prefix == name && !info.Mode().IsRegular()) {
			return nil, fs.ErrInvalid
		}
	}
	return dir.Open(name)
}

func ReadBody(root, kind, dir, file string) ([]byte, error) {
	dir = strings.TrimSuffix(strings.TrimPrefix(dir, "./"), "/")
	parts := strings.Split(dir, "/")
	if len(parts) != 2 || parts[0] != kind || !validPath(dir) {
		return nil, fmt.Errorf("invalid %s directory %q", kind, dir)
	}
	if !validPath(file) || !strings.EqualFold(path.Ext(file), ".md") {
		return nil, fmt.Errorf("invalid Markdown filename %q", file)
	}
	f, err := Open(root, dir+"/"+file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func BodyHash(root, kind, dir, file string) (string, error) {
	raw, err := ReadBody(root, kind, dir, file)
	if err != nil {
		return "", err
	}
	return Hash(raw), nil
}

func Hash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

var inlineImage = regexp.MustCompile(`(!\[[^\]]*\])\(([^)]+)\)`)

func RenderBody(raw []byte, dir string) string {
	return inlineImage.ReplaceAllStringFunc(string(raw), func(match string) string {
		m := inlineImage.FindStringSubmatch(match)
		if strings.HasPrefix(m[2], "./") {
			return m[1] + "(/static/" + path.Clean(dir) + "/" + strings.TrimPrefix(m[2], "./") + ")"
		}
		return match
	})
}

// AssetRef resolves a published Markdown image or cover.
func AssetRef(ref, dir string) string {
	u, err := url.Parse(ref)
	if err != nil || u.IsAbs() || u.Host != "" {
		return ""
	}
	if strings.HasPrefix(u.Path, "/static/") {
		return strings.TrimPrefix(u.Path, "/static/")
	}
	rel := strings.TrimPrefix(u.Path, "./")
	if !validPath(rel) {
		return ""
	}
	return path.Clean(dir) + "/" + rel
}

func ReferencesImage(raw []byte, dir, asset string) bool {
	for _, m := range inlineImage.FindAllSubmatch(raw, -1) {
		if AssetRef(string(m[2]), dir) == asset {
			return true
		}
	}
	return false
}

// StoredPaths includes the relative paths used by earlier versions.
func StoredPaths(dir string) []string { return []string{dir, dir + "/", "./" + dir, "./" + dir + "/"} }
