package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, source string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".github", "workflows", "ci.yml")
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}
func read(t *testing.T, p string) string {
	t.Helper()
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func TestPatchSelectedJobAndIdempotent(t *testing.T) {
	p := fixture(t, "jobs:\n  lint:\n    runs-on: ubuntu-latest\n  test:\n    runs-on: ubuntu-latest\n")
	o := Options{Label: "repo-r14r-test", OS: "Linux", Jobs: []string{"lint"}}
	n, e := Patch([]string{p}, o)
	if e != nil || n != 1 {
		t.Fatalf("%d %v", n, e)
	}
	if !strings.Contains(read(t, p), "lint:\n    runs-on: [self-hosted, Linux, X64, repo-r14r-test]") || !strings.Contains(read(t, p), "test:\n    runs-on: ubuntu-latest") {
		t.Fatal(read(t, p))
	}
	n, e = Patch([]string{p}, o)
	if e != nil || n != 0 {
		t.Fatalf("repeat %d %v", n, e)
	}
}
func TestPatchRejectsUnsafeDynamicAndCrossOS(t *testing.T) {
	cases := []struct {
		body   string
		reason string
	}{
		{"jobs:\n  test:\n    runs-on: ${{ matrix.os }}\n", "dynamic"},
		{"jobs:\n  test:\n    runs-on:\n      - ubuntu-latest\n", "multiline"},
		{"jobs:\n  test:\n    runs-on: macos-latest\n", "allow-cross-os"},
		{"jobs:\n  test:\n    runs-on: ubuntu-latest # comment\n", "inline"},
	}
	for _, c := range cases {
		p := fixture(t, c.body)
		_, e := Patch([]string{p}, Options{Label: "repo-x", OS: "Linux"})
		if e == nil || !strings.Contains(e.Error(), c.reason) {
			t.Errorf("%q: %v", c.reason, e)
		}
		if read(t, p) != c.body {
			t.Errorf("mutated invalid file")
		}
	}
}
func TestPatchPrevalidatesAllFiles(t *testing.T) {
	p := fixture(t, "jobs:\n  lint:\n    runs-on: ubuntu-latest\n")
	q := filepath.Join(filepath.Dir(p), "other.yaml")
	original := "jobs:\n  test:\n    runs-on: ${{ matrix.os }}\n"
	if e := os.WriteFile(q, []byte(original), 0644); e != nil {
		t.Fatal(e)
	}
	_, e := Patch([]string{p, q}, Options{Label: "repo-x", OS: "Linux"})
	if e == nil {
		t.Fatal("accepted invalid second file")
	}
	if strings.Contains(read(t, p), "self-hosted") {
		t.Fatal("partial update occurred")
	}
}
func TestPatchMissingJob(t *testing.T) {
	p := fixture(t, "jobs:\n  lint:\n    runs-on: ubuntu-latest\n")
	_, e := Patch([]string{p}, Options{Label: "repo-x", OS: "Linux", Jobs: []string{"missing"}})
	if e == nil {
		t.Fatal("missing job accepted")
	}
}
func TestPatchRefusesSymlinks(t *testing.T) {
	p := fixture(t, "jobs:\n  lint:\n    runs-on: ubuntu-latest\n")
	link := filepath.Join(filepath.Dir(p), "sym.yaml")
	if e := os.Symlink(p, link); e != nil {
		t.Skip(e)
	}
	_, e := Patch([]string{link}, Options{Label: "repo-x", OS: "Linux"})
	if e == nil {
		t.Fatal("symlink accepted")
	}
}
func TestPatchPreservesContent(t *testing.T) {
	p := fixture(t, "jobs:\r\n  lint:\r\n    runs-on: ubuntu-latest\r\n    steps:\r\n      - run: echo runs-on: ubuntu-latest\r\n")
	_, e := Patch([]string{p}, Options{Label: "repo-x", OS: "Linux"})
	if e != nil {
		t.Fatal(e)
	}
	got := read(t, p)
	if !strings.Contains(got, "echo runs-on: ubuntu-latest\r\n") {
		t.Fatal("changed step")
	}
	if !strings.Contains(got, "repo-x]\r\n") {
		t.Fatal("lost CRLF")
	}
}
