package main

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSQLiteDsn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		options map[string]string
		wantOpt string
		wantVal string
	}{
		{
			name:    "absolute path",
			path:    filepath.Join(t.TempDir(), "test.db"),
			options: map[string]string{},
		},
		{
			name:    "path with spaces",
			path:    filepath.Join(t.TempDir(), "my database.db"),
			options: map[string]string{"mode": "rw"},
			wantOpt: "mode",
			wantVal: "rw",
		},
		{
			name: "multiple options",
			path: filepath.Join(t.TempDir(), "test.db"),
			options: map[string]string{
				"mode": "rw",
				"foo":  "bar",
			},
			wantOpt: "foo",
			wantVal: "bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sqliteDsn(tt.path, tt.options)
			if err != nil {
				t.Fatalf("sqliteDsn() error = %v", err)
			}

			u, err := url.Parse(got)
			if err != nil {
				t.Fatalf("url.Parse() error = %v", err)
			}

			if u.Scheme != "file" {
				t.Errorf("scheme = %q, want %q", u.Scheme, "file")
			}

			path := u.Path
			if runtime.GOOS == "windows" {
				var hasPrefix bool
				path, hasPrefix = strings.CutPrefix(u.Path, "/")
				if !hasPrefix {
					t.Errorf("%q must have / prefix", u.Path)
				}
			}

			if !filepath.IsAbs(filepath.FromSlash(path)) {
				t.Errorf("path %q is not absolute", path)
			}

			if tt.wantOpt != "" {
				if got := u.Query().Get(tt.wantOpt); got != tt.wantVal {
					t.Errorf(
						"query[%q] = %q, want %q",
						tt.wantOpt, got, tt.wantVal,
					)
				}
			}
		})
	}
}
