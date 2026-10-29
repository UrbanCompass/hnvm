package install

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// safeJoin places name (with the first strip components dropped) under root, or reports that it
// would land outside it. skip is true for entries that strip removes entirely.
func safeJoin(root, name string, strip int) (target string, skip bool, err error) {
	parts := strings.Split(strings.TrimPrefix(filepath.ToSlash(name), "./"), "/")
	if len(parts) <= strip {
		return "", true, nil
	}
	target = filepath.Join(root, filepath.Join(parts[strip:]...))
	if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
		return "", false, fmt.Errorf("unsafe path in archive: %q", name)
	}
	return target, target == root, nil
}

func writeFile(target string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// extractTarGz unpacks r into dest, dropping the first `strip` path components. Entries that would
// land outside dest are rejected.
func extractTarGz(r io.Reader, dest string, strip int) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()

	root, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target, skip, err := safeJoin(root, hdr.Name, strip)
		if err != nil {
			return err
		}
		if skip {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeFile(target, tr, hdr.FileInfo().Mode().Perm()); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			// Node's npm/npx are relative symlinks into lib/node_modules. Reject ones that escape.
			resolved := filepath.Join(filepath.Dir(target), hdr.Linkname)
			if filepath.IsAbs(hdr.Linkname) || !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
				return fmt.Errorf("unsafe symlink in archive: %q -> %q", hdr.Name, hdr.Linkname)
			}
			os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		}
	}
}

// extractZipFile copies the single entry named `name` out of the zip at path into target.
func extractZipFile(path, name, target string) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		return writeFile(target, rc, 0o755)
	}
	return fmt.Errorf("%s not found in archive", name)
}
