package update

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func zipped(t *testing.T, names []string, body []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, n := range names {
		f, e := w.Create(n)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.Write(body); e != nil {
			t.Fatal(e)
		}
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestVersions(t *testing.T) {
	for _, x := range []string{"1.2.3", "v1.2.3", "0.0.0"} {
		if _, e := Version(x); e != nil {
			t.Fatal(e)
		}
	}
	for _, x := range []string{"../1", "1.0", "1.0.0-alpha", "01.2.3", "1.2.3;echo hi"} {
		if _, e := Version(x); e == nil {
			t.Fatalf("accepted %s", x)
		}
	}
	c, e := CompareVersion("1.10.0", "1.9.9")
	if e != nil || c != 1 {
		t.Fatal(c, e)
	}
}
func TestRepositoryValidation(t *testing.T) {
	for _, x := range []string{"nuitsjp/ucspec", "org/repo.x"} {
		if e := ValidateRepository(x); e != nil {
			t.Fatal(e)
		}
	}
	for _, x := range []string{"https://github.com/a/b", "../foo", "a/../../b", "a/b/c", "a b/c"} {
		if ValidateRepository(x) == nil {
			t.Fatalf("accepted %s", x)
		}
	}
}
func TestChecksum(t *testing.T) {
	b := []byte("binary")
	line := fmt.Sprintf("%x  tool.zip\n", sha256.Sum256(b))
	if e := VerifyChecksum(b, []byte(line), "tool.zip"); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{"", line + line, "nothex tool.zip"} {
		if VerifyChecksum(b, []byte(bad), "tool.zip") == nil {
			t.Fatal("bad manifest accepted")
		}
	}
	if VerifyChecksum([]byte("tampered"), []byte(line), "tool.zip") == nil {
		t.Fatal("tampered binary accepted")
	}
}
func TestArchiveSafety(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute", "C:/drive", `..\escape`} {
		b := zipped(t, []string{"ucspec.exe", name}, []byte("data"))
		if _, e := ExtractExecutable(b); e == nil {
			t.Fatalf("accepted %s", name)
		}
	}
	for _, names := range [][]string{{"nested/ucspec.exe"}, {"ucspec.exe", "ucspec.exe"}} {
		if _, e := ExtractExecutable(zipped(t, names, []byte("data"))); e == nil {
			t.Fatal(names)
		}
	}
	b, e := ExtractExecutable(zipped(t, []string{"ucspec.exe", "LICENSE"}, []byte("data")))
	if e != nil || string(b) != "data" {
		t.Fatal(e)
	}
	if ValidatePE(b) == nil {
		t.Fatal("non-PE accepted")
	}
}
func TestArchiveSymlink(t *testing.T) {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	h := &zip.FileHeader{Name: "ucspec.exe"}
	h.SetMode(os.ModeSymlink | 0777)
	f, _ := w.CreateHeader(h)
	f.Write([]byte("target"))
	w.Close()
	if _, e := ExtractExecutable(b.Bytes()); e == nil {
		t.Fatal("symlink accepted")
	}
}
func TestResolveRelease(t *testing.T) {
	r := Release{Tag: "v1.2.0", Assets: []Asset{{ID: 1, Name: "ucspec_1.2.0_windows_amd64.zip"}, {ID: 2, Name: "checksums.txt"}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/repos/a/b/releases/latest" && req.URL.Path != "/repos/a/b/releases/tags/v1.2.0" {
			http.NotFound(w, req)
			return
		}
		if req.Header.Get("Authorization") != "Bearer test" {
			t.Error("missing token")
		}
		json.NewEncoder(w).Encode(r)
	}))
	defer server.Close()
	c := &Client{API: server.URL, HTTP: server.Client(), Token: "test"}
	p, e := c.Resolve(context.Background(), "a/b", "")
	if e != nil || p.Version != "1.2.0" {
		t.Fatal(p, e)
	}
	_, e = c.Resolve(context.Background(), "a/b", "1.2.0")
	if e != nil {
		t.Fatal(e)
	}
	r.Prerelease = true
	if _, e = c.Resolve(context.Background(), "a/b", ""); e == nil {
		t.Fatal("prerelease accepted")
	}
	r.Prerelease = false
	r.Assets = append(r.Assets, r.Assets[0])
	if _, e = c.Resolve(context.Background(), "a/b", ""); e == nil {
		t.Fatal("duplicate asset accepted")
	}
}
func TestHTTPErrorAndLimit(t *testing.T) {
	status := 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status); fmt.Fprint(w, "0123456789") }))
	defer server.Close()
	c := &Client{API: server.URL, HTTP: server.Client()}
	if _, e := c.get(context.Background(), "/", 2, false); e == nil {
		t.Fatal("HTTP error ignored")
	}
	status = 200
	if _, e := c.get(context.Background(), "/", 2, false); e == nil {
		t.Fatal("size limit ignored")
	}
}
func TestRedirectPolicy(t *testing.T) {
	c := NewClient("secret")
	for _, s := range []string{"http://api.github.com/asset", "https://evil.invalid/asset", "https://user@github.com/asset"} {
		u, _ := url.Parse(s)
		req := &http.Request{URL: u, Header: http.Header{}}
		if c.HTTP.CheckRedirect(req, nil) == nil {
			t.Fatal("unsafe redirect accepted")
		}
	}
	u, _ := url.Parse("https://release-assets.githubusercontent.com/a")
	req := &http.Request{URL: u, Header: http.Header{"Authorization": []string{"Bearer secret"}}}
	if e := c.HTTP.CheckRedirect(req, nil); e != nil || req.Header.Get("Authorization") != "" {
		t.Fatal("token leaked")
	}
}
func TestReplaceAndLock(t *testing.T) {
	f := filepath.Join(t.TempDir(), "tool.exe")
	os.WriteFile(f, []byte("old"), 0700)
	backup, e := Replace(f, []byte("new"))
	if e != nil {
		t.Fatal(e)
	}
	old, _ := os.ReadFile(backup)
	now, _ := os.ReadFile(f)
	if string(old) != "old" || string(now) != "new" {
		t.Fatal("wrong replacement")
	}
	os.WriteFile(f+".update.lock", nil, 0600)
	if _, e = Replace(f, []byte("bad")); e == nil {
		t.Fatal("lock ignored")
	}
	now, _ = os.ReadFile(f)
	if string(now) != "new" {
		t.Fatal("locked file altered")
	}
}
func TestProjectUpdatePinsOnlyAfterInstall(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, ".ucspec-version"), []byte("0.1.0\n"), 0600)
	path, e := InstallProject(root, "0.2.0", []byte("new"))
	if e != nil {
		t.Fatal(e)
	}
	p, _ := os.ReadFile(filepath.Join(root, ".ucspec-version"))
	b, _ := os.ReadFile(path)
	if string(p) != "0.2.0\n" || string(b) != "new" {
		t.Fatal("wrong install")
	}
	if _, e = InstallProject(root, "0.2.0", []byte("different")); e == nil {
		t.Fatal("existing version overwritten")
	}
	p, _ = os.ReadFile(filepath.Join(root, ".ucspec-version"))
	if string(p) != "0.2.0\n" {
		t.Fatal("pin corrupted")
	}
	if !IsProjectExecutable(path) {
		t.Fatal("pinned executable not detected")
	}
}
func TestReplaceSymlink(t *testing.T) {
	d := t.TempDir()
	f := filepath.Join(d, "target")
	os.WriteFile(f, []byte("old"), 0600)
	link := filepath.Join(d, "link")
	if err := os.Symlink(f, link); err != nil {
		t.Skip("OS does not allow symlink")
	}
	if _, e := Replace(link, []byte("new")); e == nil {
		t.Fatal("symlink followed")
	}
}
