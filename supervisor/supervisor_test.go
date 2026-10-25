package main

import (
	"os"
	"os/user"
	"strings"
	"testing"
)

func TestCheckCosmosName(t *testing.T) {
	valid := []string{"cosmos", "Cosmos_1", "a.b_c", "0"}
	invalid := []string{"", "cosmos name", "cosmos/name", "cosmos-1", "宇宙"}
	for _, s := range valid {
		if !checkCosmosName(s) {
			t.Errorf("checkCosmosName(%q) = false, want true", s)
		}
	}
	for _, s := range invalid {
		if checkCosmosName(s) {
			t.Errorf("checkCosmosName(%q) = true, want false", s)
		}
	}
}

func TestCheckNodeName(t *testing.T) {
	valid := []string{"node", "Node_1", "n.b_c", "0"}
	invalid := []string{"", "node name", "node/name", "node-1", "节点"}
	for _, s := range valid {
		if !checkNodeName(s) {
			t.Errorf("checkNodeName(%q) = false, want true", s)
		}
	}
	for _, s := range invalid {
		if checkNodeName(s) {
			t.Errorf("checkNodeName(%q) = true, want false", s)
		}
	}
}

func currentUserAndGroup(t *testing.T) (*user.User, *user.Group) {
	t.Helper()
	u, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current: %v", err)
	}
	g, err := user.LookupGroupId(u.Gid)
	if err != nil {
		t.Fatalf("user.LookupGroupId(%s): %v", u.Gid, err)
	}
	return u, g
}

func TestPathCreateDirectoryIfNotExist(t *testing.T) {
	u, g := currentUserAndGroup(t)
	dir := t.TempDir()
	p := NewPath(dir + "/atomos_test_dir")

	if err := p.CreateDirectoryIfNotExist(u, g, 0775); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := p.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if !p.IsDir() {
		t.Fatal("created path is not a directory")
	}
	if p.Mode().Perm() != 0775 {
		t.Fatalf("mode = %o, want 775", p.Mode().Perm())
	}

	// Idempotent: second create confirms instead of failing.
	if err := p.CreateDirectoryIfNotExist(u, g, 0775); err != nil {
		t.Fatalf("second create: %v", err)
	}

	// A file where a directory is expected must fail.
	f := NewPath(dir + "/atomos_test_file")
	if err := os.WriteFile(f.path, []byte("x"), 0644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := f.CreateDirectoryIfNotExist(u, g, 0775); err == nil {
		t.Fatal("expected error creating directory over a file")
	}
}

func TestPathCheckDirectoryOwnerAndMode(t *testing.T) {
	u, _ := currentUserAndGroup(t)
	dir := t.TempDir()
	p := NewPath(dir)
	if err := p.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if err := p.CheckDirectoryOwnerAndMode(u, p.Mode().Perm()); err != nil {
		t.Fatalf("check actual TempDir mode %o: %v", p.Mode().Perm(), err)
	}
	if err := p.CheckDirectoryOwnerAndMode(u, p.Mode().Perm()^0001); err == nil {
		t.Fatal("expected mode mismatch error")
	}

	// A directory we create with the exact mode must pass.
	sub := NewPath(dir + "/checked")
	if err := os.Mkdir(sub.path, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chmod(sub.path, 0755); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if err := sub.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if err := sub.CheckDirectoryOwnerAndMode(u, 0755); err != nil {
		t.Fatalf("check: %v", err)
	}

	// Missing directory fails cleanly.
	missing := NewPath(dir + "/nope")
	if err := missing.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if err := missing.CheckDirectoryOwnerAndMode(u, 0755); err == nil || !strings.Contains(err.Error(), "not exists") {
		t.Fatalf("missing dir: got %v, want not-exists error", err)
	}
}

func TestPathCreateFileIfNotExist(t *testing.T) {
	dir := t.TempDir()
	p := NewPath(dir + "/supervisor.conf")
	if err := p.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if err := p.CreateFileIfNotExist([]byte("cosmos-name: c\n"), 0644); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Second call must NOT overwrite existing content.
	if err := p.CreateFileIfNotExist([]byte("cosmos-name: other\n"), 0644); err != nil {
		t.Fatalf("second create: %v", err)
	}
	buf, err := os.ReadFile(p.path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "cosmos-name: c\n" {
		t.Fatalf("content overwritten: %q", buf)
	}
}

func TestPathCreateFileIfNotExistConcurrent(t *testing.T) {
	p := NewPath(t.TempDir() + "/supervisor.conf")
	const writers = 16
	start := make(chan struct{})
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		go func() {
			<-start
			errs <- p.CreateFileIfNotExist([]byte("cosmos-name: c\n"), 0644)
		}()
	}
	close(start)
	for i := 0; i < writers; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent create: %v", err)
		}
	}
	buf, err := os.ReadFile(p.path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "cosmos-name: c\n" {
		t.Fatalf("content = %q", buf)
	}
}

func TestInitPathsRequiresRoot(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; the non-root rejection path cannot trigger")
	}
	err := initPaths("u", "g", "cosmos", []string{"node"})
	if err == nil || !strings.Contains(err.Error(), "root") {
		t.Fatalf("initPaths as non-root: got %v, want root requirement error", err)
	}
}
