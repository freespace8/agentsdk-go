package security

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestSandboxValidatePath(t *testing.T) {
	root := tempDirClean(t)
	inside := filepath.Join(root, "dir", "file.txt")
	if err := os.MkdirAll(filepath.Dir(inside), 0o755); err != nil {
		t.Fatalf("mk inside: %v", err)
	}
	if err := os.WriteFile(inside, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	outsideRoot := tempDirClean(t)
	outside := filepath.Join(outsideRoot, "escape.txt")
	if err := os.WriteFile(outside, []byte("nope"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		allow   string
		wantErr string
	}{
		{"inside root allowed", inside, "", ""},
		{"outside root blocked", outside, "", ErrPathNotAllowed.Error()},
		{"additional allowlist enables path", outside, outsideRoot, ""},
		{"empty path rejected", "   ", "", "empty path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sandbox := NewSandbox(root)
			if tt.allow != "" {
				sandbox.Allow(tt.allow)
			}
			err := sandbox.ValidatePath(tt.path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestSandboxRejectsSymlinkEscape(t *testing.T) {
	tests := []struct {
		name     string
		wantErr  string
		makePath func(t *testing.T, root string) string
	}{
		{
			name:    "symlink outside root rejected",
			wantErr: "symlink",
			makePath: func(t *testing.T, root string) string {
				t.Helper()
				target := filepath.Join(t.TempDir(), "target.txt")
				if err := os.WriteFile(target, []byte("outside"), 0o644); err != nil {
					t.Fatalf("write target: %v", err)
				}
				symlink := filepath.Join(root, "link.txt")
				mustSymlink(t, target, symlink)
				return symlink
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			path := tt.makePath(t, root)
			sb := NewSandbox(root)
			if err := sb.ValidatePath(path); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q got %v", tt.wantErr, err)
			}
		})
	}
}

func tempDirClean(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	realDir, err := filepath.EvalSymlinks(dir)
	if err == nil {
		return realDir
	}
	return dir
}

func mustSymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		if os.IsPermission(err) || errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.ENOSYS) {
			t.Skipf("symlink unsupported: %v", err)
		}
		t.Fatalf("symlink: %v", err)
	}
}
