//go:build windows

package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Run only on Windows CI. This checks replacing the image of the running process,
// not merely replacing an unlocked dummy file on another operating system.
func TestReplaceRunningWindowsImage(t *testing.T) {
	if target := os.Getenv("UCSPEC_TEST_REPLACE_IMAGE"); target != "" {
		b, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = Replace(target, b); err != nil {
			t.Fatal(err)
		}
		return
	}
	src, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "running test.exe")
	if err = os.WriteFile(target, b, 0700); err != nil {
		t.Fatal(err)
	}
	c := exec.Command(target, "-test.run=^TestReplaceRunningWindowsImage$")
	c.Env = append(os.Environ(), "UCSPEC_TEST_REPLACE_IMAGE="+target)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, err = os.Stat(target + ".previous"); err != nil {
		t.Fatal(err)
	}
}
