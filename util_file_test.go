package atomos

import (
	"io"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestUtilFileCreateFileThenCreateFileTruncate(t *testing.T) {
	now := time.Now()
	tmpTestFilepath := path.Join(os.TempDir(), "go-atomos-test-"+now.Format("20060102150405"))

	defer func() {
		// Clean up
		if er := os.Remove(tmpTestFilepath); er != nil {
			t.Fatalf("UtilFileCreateFile() cleanup = %v, want %v", er, nil)
		}
	}()

	// Test create file truncate
	f, er := os.OpenFile(tmpTestFilepath, os.O_CREATE|os.O_WRONLY, 0664)
	if er != nil {
		t.Fatalf("UtilFileCreateFile() setup = %v, want %v", er, nil)
	}
	f.Write([]byte("testdata"))
	f.Close()

	fTruncate, er := os.OpenFile(tmpTestFilepath, os.O_TRUNC, 0664)
	if er != nil {
		t.Fatalf("UtilFileCreateFile() = %v, want %v", er, nil)
	}
	bufTruncate, er := io.ReadAll(fTruncate)
	if er != nil {
		t.Fatalf("UtilFileCreateFile() = %v, want %v", er, nil)
	}
	if len(bufTruncate) != 0 {
		t.Errorf("UtilFileCreateFile() = %v, want %v", string(bufTruncate), "")
	}
	fTruncate.Close()
}

func TestUtilFileCreateFileThenCreateFileExclusiveFailed(t *testing.T) {
	now := time.Now()
	tmpTestFilepath := path.Join(os.TempDir(), "go-atomos-test-"+now.Format("20060102150405"))

	defer func() {
		// Clean up
		if er := os.Remove(tmpTestFilepath); er != nil {
			t.Fatalf("UtilFileCreateFile() cleanup = %v, want %v", er, nil)
		}
	}()

	// Test create file exclusive failed
	f, er := os.OpenFile(tmpTestFilepath, os.O_CREATE|os.O_EXCL, 0664)
	if er != nil {
		t.Fatalf("UtilFileCreateFile() setup = %v, want %v", er, nil)
	}
	f.Close()

	fExcl, er := os.OpenFile(tmpTestFilepath, os.O_CREATE|os.O_EXCL, 0664)
	if er == nil {
		t.Errorf("UtilFileCreateFile() = %v, want %v", er, "error")
	}
	if fExcl != nil {
		t.Errorf("UtilFileCreateFile() = %v, want %v", fExcl, "error")
	}
}

func TestUtilFileEnsureDirectory(t *testing.T) {
	type args struct {
		dir            string
		perm           os.FileMode
		ensureWritable bool

		prev, post func()
	}
	tests := []struct {
		name    string
		args    args
		wantErr string
	}{
		{
			name: "Test UtilFileEnsureDirectory",
			args: args{
				dir:            path.Join(os.TempDir(), "test"),
				perm:           0774,
				ensureWritable: true,
			},
			wantErr: "",
		},
		{
			name: "Test UtilFileEnsureDirectory",
			args: args{
				dir:            path.Join(os.TempDir(), "test"),
				perm:           0774,
				ensureWritable: false,
			},
			wantErr: "",
		},
		{
			name: "Test UtilFileEnsureDirectory file exists",
			args: args{
				dir:            path.Join(os.TempDir(), "test"),
				perm:           0774,
				ensureWritable: true,
				prev: func() {
					if er := os.MkdirAll(path.Join(os.TempDir(), "test"), 0774); er != nil {
						t.Errorf("UtilFileEnsureDirectory() = %v, want %v", er, nil)
					}
				},
			},
		},
		{
			name: "Test UtilFileEnsureDirectory file exists and is not directory",
			args: args{
				dir:            path.Join(os.TempDir(), "test"),
				perm:           0774,
				ensureWritable: true,
				prev: func() {
					if er := os.WriteFile(path.Join(os.TempDir(), "test"), []byte{}, 0774); er != nil {
						t.Errorf("UtilFileEnsureDirectory() = %v, want %v", er, nil)
					}
				},
			},
			wantErr: "UtilFileEnsureDirectory: Path is not directory. dir=",
		},
		{
			name: "Test UtilFileEnsureDirectory permission denied",
			args: args{
				dir:            path.Join(os.TempDir(), "test"),
				perm:           0664,
				ensureWritable: true,
			},
			wantErr: "UtilFileEnsureDirectory: Cannot write test file. dir=",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.args.prev != nil {
				tt.args.prev()
			}

			got := UtilFileEnsureDirectory(tt.args.dir, tt.args.perm, tt.args.ensureWritable)
			if got == nil {
				if tt.wantErr != "" {
					t.Errorf("UtilFileEnsureDirectory() = %v, want %v", got, tt.wantErr)
				}
			} else {
				if tt.wantErr == "" {
					t.Errorf("UtilFileEnsureDirectory() = %v, want %v", got, nil)
				} else if !strings.Contains(got.Error(), tt.wantErr) {
					t.Errorf("UtilFileEnsureDirectory() = %v, want %v", got, tt.wantErr)
				}
			}
			if er := os.RemoveAll(tt.args.dir); er != nil {
				t.Errorf("UtilFileEnsureDirectory() = %v, want %v", er, nil)
			}

			if tt.args.post != nil {
				tt.args.post()
			}
		})
	}
}

func TestUtilFileGetDirectorySize(t *testing.T) {
	type args struct {
		dir string

		prev, post func()
	}
	tests := []struct {
		name  string
		args  args
		want  int64
		want1 *Error
	}{
		{
			name: "Test UtilFileGetDirectorySize",
			args: args{
				dir: path.Join(os.TempDir(), "test"),
				prev: func() {
					if er := os.MkdirAll(path.Join(os.TempDir(), "test"), 0774); er != nil {
						t.Errorf("UtilFileGetDirectorySize() = %v, want %v", er, nil)
					}
				},
				post: func() {
					if er := os.RemoveAll(path.Join(os.TempDir(), "test")); er != nil {
						t.Errorf("UtilFileGetDirectorySize() = %v, want %v", er, nil)
					}
				},
			},
			want: 64,
		},
		{
			name: "Test UtilFileGetDirectorySize",
			args: args{
				dir: path.Join(os.TempDir(), "test"),
				prev: func() {
					if er := os.MkdirAll(path.Join(os.TempDir(), "test"), 0774); er != nil {
						t.Errorf("UtilFileGetDirectorySize() = %v, want %v", er, nil)
					}
					gen := NewUtilStringRandomStringGenerator()
					for i := 0; i < 10; i++ {
						buf := []byte(gen.RandomString(1000))
						if er := os.WriteFile(path.Join(os.TempDir(), "test", "test_"+strconv.Itoa(i)), buf, 0774); er != nil {
							t.Errorf("UtilFileGetDirectorySize() = %v, want %v", er, nil)
						}
					}
				},
				post: func() {
					if er := os.RemoveAll(path.Join(os.TempDir(), "test")); er != nil {
						t.Errorf("UtilFileGetDirectorySize() = %v, want %v", er, nil)
					}
				},
			},
			want: 64 + (1000+32)*10,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.args.prev != nil {
				tt.args.prev()
			}

			got, got1 := UtilFileGetDirectorySize(tt.args.dir)
			if got != tt.want {
				t.Errorf("UtilFileGetDirectorySize() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("UtilFileGetDirectorySize() got1 = %v, want %v", got1, tt.want1)
			}

			if tt.args.post != nil {
				tt.args.post()
			}
		})
	}
}
