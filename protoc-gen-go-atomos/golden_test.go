package main

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

var update = flag.Bool("update", false, "update golden files")

// TestGolden parses testdata/golden/golden.proto with protoc (descriptor set),
// drives the plugin's code generation in-process via protogen, and compares
// the generated _atomos.pb.go against the checked-in golden file.
// It requires protoc on PATH; no plugin binary or go toolchain subprocess is
// needed.
func TestGolden(t *testing.T) {
	protoc, err := exec.LookPath("protoc")
	if err != nil {
		t.Skip("protoc not found on PATH")
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Dir(wd)
	protoDir := filepath.Join(wd, "testdata", "golden")

	tmp := t.TempDir()
	descOut := filepath.Join(tmp, "golden.protoset")
	cmd := exec.Command(protoc,
		"-I", protoDir,
		"-I", repoRoot, // for the atomos.proto import
		"--include_imports",
		"--include_source_info",
		"--descriptor_set_out="+descOut,
		"golden.proto",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("protoc descriptor set: %v\n%s", err, out)
	}

	raw, err := os.ReadFile(descOut)
	if err != nil {
		t.Fatal(err)
	}
	var fds descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(raw, &fds); err != nil {
		t.Fatalf("unmarshal descriptor set: %v", err)
	}

	req := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{"golden.proto"},
		ProtoFile:      fds.File,
		Parameter:      proto.String("paths=source_relative"),
	}
	gen, err := protogen.Options{}.New(req)
	if err != nil {
		t.Fatalf("protogen.New: %v", err)
	}
	for _, f := range gen.Files {
		if !f.Generate {
			continue
		}
		generateFile(gen, f)
	}
	resp := gen.Response()
	if resp.Error != nil {
		t.Fatalf("generation error: %s", resp.GetError())
	}
	var got string
	for _, f := range resp.File {
		if f.GetName() == "golden_atomos.pb.go" {
			got = f.GetContent()
		}
	}
	if got == "" {
		t.Fatal("plugin produced no golden_atomos.pb.go")
	}

	goldenPath := filepath.Join(protoDir, "golden_atomos.pb.go.golden")
	if *update {
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("update golden: %v", err)
		}
		t.Logf("updated %s", goldenPath)
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if got != string(want) {
		t.Errorf("generated output does not match golden file %s (run with -update to refresh)", goldenPath)
	}
}
