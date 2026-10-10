// Package update installs explicitly requested, checksum-verified GitHub release binaries.
// No background update, arbitrary script execution, or certificate bypass is supported.
package update

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"debug/pe"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const MaxArchive = 64 << 20

var repositoryRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*/[A-Za-z0-9][A-Za-z0-9_.-]*$`)
var versionRE = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func ValidateRepository(repo string) error {
	if !repositoryRE.MatchString(repo) || strings.Contains(repo, "..") {
		return fmt.Errorf("repository must be owner/name")
	}
	return nil
}
func Version(v string) ([3]uint64, error) {
	var out [3]uint64
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	m := versionRE.FindStringSubmatch(v)
	if m == nil {
		return out, fmt.Errorf("stable version must be x.y.z: %q", v)
	}
	for i := range out {
		n, e := strconv.ParseUint(m[i+1], 10, 64)
		if e != nil {
			return out, e
		}
		out[i] = n
	}
	return out, nil
}
func CompareVersion(a, b string) (int, error) {
	x, e := Version(a)
	if e != nil {
		return 0, e
	}
	y, e := Version(b)
	if e != nil {
		return 0, e
	}
	for i := range x {
		if x[i] < y[i] {
			return -1, nil
		}
		if x[i] > y[i] {
			return 1, nil
		}
	}
	return 0, nil
}
func CleanVersion(v string) (string, error) {
	if _, e := Version(v); e != nil {
		return "", e
	}
	return strings.TrimPrefix(strings.TrimSpace(v), "v"), nil
}

type Asset struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}
type Release struct {
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}
type Plan struct {
	Version    string `json:"version"`
	Repository string `json:"repository"`
	Archive    Asset  `json:"archive"`
	Checksums  Asset  `json:"checksums"`
}
type Client struct {
	HTTP  *http.Client
	API   string
	Token string
}

func NewClient(token string) *Client {
	return &Client{API: "https://api.github.com", Token: token, HTTP: &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		if req.URL.Scheme != "https" || req.URL.User != nil {
			return fmt.Errorf("unsafe update redirect")
		}
		h := req.URL.Hostname()
		if h != "github.com" && h != "api.github.com" && h != "release-assets.githubusercontent.com" && h != "objects.githubusercontent.com" {
			return fmt.Errorf("unexpected update redirect host %s", h)
		}
		if h != "api.github.com" {
			req.Header.Del("Authorization")
		}
		return nil
	}}}
}
func (c *Client) get(ctx context.Context, path string, max int64, binary bool) ([]byte, error) {
	u, err := url.Parse(c.API + path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ucspec-updater")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Accept", "application/vnd.github+json")
	if binary {
		req.Header.Set("Accept", "application/octet-stream")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub update request returned HTTP %d (check repository, release, permissions and rate limits)", resp.StatusCode)
	}
	if resp.ContentLength > max {
		return nil, fmt.Errorf("release asset exceeds size limit")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("release asset exceeds size limit")
	}
	return b, nil
}
func (c *Client) Resolve(ctx context.Context, repo, requested string) (Plan, error) {
	p := Plan{Repository: repo}
	if err := ValidateRepository(repo); err != nil {
		return p, err
	}
	suffix := "latest"
	if requested != "" {
		v, e := CleanVersion(requested)
		if e != nil {
			return p, e
		}
		suffix = "tags/v" + v
	}
	b, e := c.get(ctx, "/repos/"+repo+"/releases/"+suffix, 2<<20, false)
	if e != nil {
		return p, e
	}
	var r Release
	if e = json.Unmarshal(b, &r); e != nil {
		return p, e
	}
	if r.Draft || r.Prerelease {
		return p, fmt.Errorf("only stable, published releases are accepted")
	}
	p.Version, e = CleanVersion(r.Tag)
	if e != nil {
		return p, e
	}
	if requested != "" {
		v, _ := CleanVersion(requested)
		if v != p.Version {
			return p, fmt.Errorf("release tag differs from requested version")
		}
	}
	name := "ucspec_" + p.Version + "_windows_amd64.zip"
	for _, a := range r.Assets {
		if a.ID <= 0 {
			continue
		}
		switch a.Name {
		case name:
			if p.Archive.ID != 0 {
				return p, fmt.Errorf("duplicate archive asset")
			}
			p.Archive = a
		case "checksums.txt":
			if p.Checksums.ID != 0 {
				return p, fmt.Errorf("duplicate checksum asset")
			}
			p.Checksums = a
		}
	}
	if p.Archive.ID == 0 || p.Checksums.ID == 0 {
		return p, fmt.Errorf("release needs %s and checksums.txt", name)
	}
	return p, nil
}
func (c *Client) Download(ctx context.Context, p Plan) ([]byte, error) {
	path := "/repos/" + p.Repository + "/releases/assets/"
	checks, e := c.get(ctx, path+strconv.FormatInt(p.Checksums.ID, 10), 1<<20, true)
	if e != nil {
		return nil, e
	}
	archive, e := c.get(ctx, path+strconv.FormatInt(p.Archive.ID, 10), MaxArchive, true)
	if e != nil {
		return nil, e
	}
	if e = VerifyChecksum(archive, checks, p.Archive.Name); e != nil {
		return nil, e
	}
	data, e := ExtractExecutable(archive)
	if e != nil {
		return nil, e
	}
	if e = ValidatePE(data); e != nil {
		return nil, e
	}
	return data, nil
}
func VerifyChecksum(data, manifest []byte, name string) error {
	want := ""
	for _, line := range strings.Split(string(manifest), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		filename := strings.TrimPrefix(fields[1], "*")
		if filename == name {
			if want != "" {
				return fmt.Errorf("duplicate checksum entry")
			}
			want = fields[0]
		}
	}
	h, e := hex.DecodeString(want)
	if e != nil || len(h) != sha256.Size {
		return fmt.Errorf("missing or invalid SHA-256 checksum for %s", name)
	}
	sum := sha256.Sum256(data)
	if subtle.ConstantTimeCompare(h, sum[:]) != 1 {
		return fmt.Errorf("SHA-256 checksum mismatch; executable not changed")
	}
	return nil
}
func ExtractExecutable(data []byte) ([]byte, error) {
	r, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if e != nil {
		return nil, e
	}
	var result []byte
	for _, f := range r.File {
		p := strings.ReplaceAll(f.Name, "\\", "/")
		if strings.HasPrefix(p, "/") || strings.Contains(p, ":") || p == ".." || strings.Contains(p, "../") || f.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("unsafe ZIP entry %q", f.Name)
		}
		if p != "ucspec.exe" {
			continue
		}
		if result != nil {
			return nil, fmt.Errorf("duplicate ucspec.exe")
		}
		if f.UncompressedSize64 > MaxArchive {
			return nil, fmt.Errorf("executable exceeds size limit")
		}
		rd, er := f.Open()
		if er != nil {
			return nil, er
		}
		result, er = io.ReadAll(io.LimitReader(rd, MaxArchive+1))
		closeErr := rd.Close()
		if er != nil {
			return nil, er
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if len(result) > MaxArchive {
			return nil, fmt.Errorf("executable exceeds size limit")
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("archive must contain ucspec.exe at its root")
	}
	return result, nil
}
func ValidatePE(data []byte) error {
	f, e := pe.NewFile(bytes.NewReader(data))
	if e != nil {
		return fmt.Errorf("invalid Windows PE executable: %w", e)
	}
	defer f.Close()
	if f.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
		return fmt.Errorf("expected Windows amd64 executable")
	}
	if f.Characteristics&pe.IMAGE_FILE_EXECUTABLE_IMAGE == 0 {
		return fmt.Errorf("PE image is not executable")
	}
	return nil
}

// Replace stages in the SAME directory, then renames the old image before the
// replacement. Rename failure restores the original. Power loss between renames
// still requires manual recovery from the .previous file (documented).
func Replace(target string, data []byte) (backup string, err error) {
	target, err = filepath.Abs(target)
	if err != nil {
		return "", err
	}
	fi, err := os.Lstat(target)
	if err != nil {
		return "", err
	}
	if !fi.Mode().IsRegular() {
		return "", fmt.Errorf("update target must be a regular, non-symlink file")
	}
	lockPath := target + ".update.lock"
	lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", fmt.Errorf("cannot acquire update lock: %w", err)
	}
	fmt.Fprintln(lock, os.Getpid())
	_ = lock.Close()
	defer os.Remove(lockPath)
	stage, err := os.CreateTemp(filepath.Dir(target), ".ucspec-stage-*.exe")
	if err != nil {
		return "", err
	}
	stageName := stage.Name()
	defer os.Remove(stageName)
	if _, err = stage.Write(data); err == nil {
		err = stage.Sync()
	}
	closeErr := stage.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err = os.Chmod(stageName, fi.Mode().Perm()); err != nil {
		return "", err
	}
	backup = target + ".previous"
	if _, e := os.Lstat(backup); e == nil {
		if e = os.Remove(backup); e != nil {
			return "", fmt.Errorf("cannot remove previous backup (possibly still running): %w", e)
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return "", e
	}
	if err = os.Rename(target, backup); err != nil {
		return "", fmt.Errorf("cannot rename running executable; original unchanged: %w", err)
	}
	if err = os.Rename(stageName, target); err != nil {
		if restore := os.Rename(backup, target); restore != nil {
			return backup, fmt.Errorf("replacement failed: %v; restore manually from %s: %v", err, backup, restore)
		}
		return "", fmt.Errorf("replacement failed; original restored: %w", err)
	}
	return backup, nil
}
func RequireWindows() error {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return fmt.Errorf("binary self-update supports Windows amd64 only")
	}
	return nil
}
