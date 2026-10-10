package update

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InstallProject writes a new version side by side before changing the pin.
// It never replaces the executable that is currently running.
func InstallProject(root, version string, data []byte) (string, error) {
	v, err := CleanVersion(version)
	if err != nil {
		return "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	pin := filepath.Join(root, ".ucspec-version")
	fi, err := os.Lstat(pin)
	if err != nil {
		return "", fmt.Errorf("project must already have .ucspec-version: %w", err)
	}
	if !fi.Mode().IsRegular() {
		return "", fmt.Errorf("pin must be a regular file")
	}
	dir := filepath.Join(root, ".tools", "ucspec", v)
	if err = rejectSymlinkAncestors(root, dir); err != nil {
		return "", err
	}
	if err = os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	target := filepath.Join(dir, "ucspec.exe")
	if info, statErr := os.Lstat(target); statErr == nil && !info.Mode().IsRegular() {
		return "", fmt.Errorf("installed executable must be a regular file")
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return "", statErr
	}
	if old, err := os.ReadFile(target); err == nil {
		if string(old) != string(data) {
			return "", fmt.Errorf("different executable already installed for version %s", v)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	} else {
		f, e := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0755)
		if e != nil {
			return "", e
		}
		_, e = f.Write(data)
		if e == nil {
			e = f.Sync()
		}
		ce := f.Close()
		if e != nil {
			os.Remove(target)
			return "", e
		}
		if ce != nil {
			os.Remove(target)
			return "", ce
		}
	}
	if err = writePin(pin, v+"\n"); err != nil {
		return target, err
	}
	return target, nil
}
func writePin(pin, text string) error {
	if old, err := os.ReadFile(pin); err == nil && string(old) == text {
		return nil
	}
	_, err := Replace(pin, []byte(text))
	return err
}
func rejectSymlinkAncestors(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("path escapes project")
	}
	cur := root
	for _, p := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, p)
		f, e := os.Lstat(cur)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink/reparse point: %s", cur)
		}
	}
	return nil
}
func IsProjectExecutable(path string) bool {
	p := strings.ToLower(filepath.ToSlash(path))
	return strings.Contains(p, "/.tools/ucspec/")
}
