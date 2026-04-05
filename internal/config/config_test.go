package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mkende/golink-url-shortener/internal/config"
)

// writeTemp writes content to a temporary TOML file and returns its path.
// The file is automatically removed when the test finishes.
func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "config-*.toml")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("closing temp file: %v", err)
	}
	return f.Name()
}

func TestLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		toml        string
		wantErr     bool
		errContains string
		check       func(t *testing.T, cfg *config.Config)
	}{
		{
			name: "valid minimal config",
			toml: `canonical_domain = "go.example.com"`,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				if cfg.CanonicalDomain != "go.example.com" {
					t.Errorf("CanonicalDomain = %q, want %q", cfg.CanonicalDomain, "go.example.com")
				}
			},
		},
		{
			name: "defaults applied when fields omitted",
			toml: `canonical_domain = "go.example.com"`,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				if cfg.ListenAddr != "0.0.0.0:8080" {
					t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, "0.0.0.0:8080")
				}
				if cfg.Title != "GoLink" {
					t.Errorf("Title = %q, want %q", cfg.Title, "GoLink")
				}
				if cfg.DB.Driver != "sqlite" {
					t.Errorf("DB.Driver = %q, want %q", cfg.DB.Driver, "sqlite")
				}
				if cfg.DB.DSN != "golink.db" {
					t.Errorf("DB.DSN = %q, want %q", cfg.DB.DSN, "golink.db")
				}
				if cfg.QuickLinkLength != 6 {
					t.Errorf("QuickLinkLength = %d, want 6", cfg.QuickLinkLength)
				}
				if cfg.OIDC.GroupsClaim != "groups" {
					t.Errorf("OIDC.GroupsClaim = %q, want %q", cfg.OIDC.GroupsClaim, "groups")
				}
				wantScopes := []string{"openid", "email", "profile"}
				if len(cfg.OIDC.Scopes) != len(wantScopes) {
					t.Errorf("OIDC.Scopes = %v, want %v", cfg.OIDC.Scopes, wantScopes)
				} else {
					for i, s := range wantScopes {
						if cfg.OIDC.Scopes[i] != s {
							t.Errorf("OIDC.Scopes[%d] = %q, want %q", i, cfg.OIDC.Scopes[i], s)
						}
					}
				}
			},
		},
		{
			name: "explicit values override defaults",
			toml: `
canonical_domain  = "go.corp.example.com"
listen_addr       = "127.0.0.1:9090"
title             = "Corp Links"
quick_link_length = 8

[db]
driver = "postgres"
dsn    = "host=localhost dbname=golink"

[oidc]
scopes       = ["openid", "email"]
groups_claim = "roles"
`,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				if cfg.ListenAddr != "127.0.0.1:9090" {
					t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, "127.0.0.1:9090")
				}
				if cfg.Title != "Corp Links" {
					t.Errorf("Title = %q, want %q", cfg.Title, "Corp Links")
				}
				if cfg.DB.Driver != "postgres" {
					t.Errorf("DB.Driver = %q, want %q", cfg.DB.Driver, "postgres")
				}
				if cfg.QuickLinkLength != 8 {
					t.Errorf("QuickLinkLength = %d, want 8", cfg.QuickLinkLength)
				}
				if cfg.OIDC.GroupsClaim != "roles" {
					t.Errorf("OIDC.GroupsClaim = %q, want %q", cfg.OIDC.GroupsClaim, "roles")
				}
				if len(cfg.OIDC.Scopes) != 2 {
					t.Errorf("len(OIDC.Scopes) = %d, want 2", len(cfg.OIDC.Scopes))
				}
			},
		},
		{
			name:        "missing canonical_domain returns error",
			toml:        `listen_addr = "0.0.0.0:8080"`,
			wantErr:     true,
			errContains: "canonical_domain is required",
		},
		{
			name:        "bad TOML syntax returns error",
			toml:        `canonical_domain = [[[`,
			wantErr:     true,
			errContains: "parsing config file",
		},
		{
			name: "quick_link_length exactly 4 is valid",
			toml: `
canonical_domain  = "go.example.com"
quick_link_length = 4
`,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				if cfg.QuickLinkLength != 4 {
					t.Errorf("QuickLinkLength = %d, want 4", cfg.QuickLinkLength)
				}
			},
		},
		{
			name:        "quick_link_length less than 4 returns error",
			toml:        `canonical_domain = "go.example.com"` + "\nquick_link_length = 3",
			wantErr:     true,
			errContains: "quick_link_length must be >= 4",
		},
		{
			name:        "quick_link_length of 1 returns error",
			toml:        `canonical_domain = "go.example.com"` + "\nquick_link_length = 1",
			wantErr:     true,
			errContains: "quick_link_length must be >= 4",
		},
		{
			name:        "invalid db driver returns error",
			toml:        "canonical_domain = \"go.example.com\"\n[db]\ndriver = \"mysql\"",
			wantErr:     true,
			errContains: "db.driver must be",
		},
		{
			name: "admin fields parsed correctly",
			toml: `
canonical_domain = "go.example.com"
admin_emails     = ["alice@example.com", "bob@example.com"]
admin_group      = "sre"
`,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				if len(cfg.AdminEmails) != 2 {
					t.Errorf("len(AdminEmails) = %d, want 2", len(cfg.AdminEmails))
				}
				if cfg.AdminGroup != "sre" {
					t.Errorf("AdminGroup = %q, want %q", cfg.AdminGroup, "sre")
				}
			},
		},
		{
			name:        "non-existent file returns error",
			toml:        "", // not used — we pass a bad path instead
			wantErr:     true,
			errContains: "reading config file",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var path string
			if tc.errContains == "reading config file" {
				// Use a path that does not exist.
				path = filepath.Join(t.TempDir(), "nonexistent.toml")
			} else {
				path = writeTemp(t, tc.toml)
			}

			cfg, err := config.Load(path)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("Load() returned nil error, want error containing %q", tc.errContains)
				}
				if tc.errContains != "" {
					if got := err.Error(); !containsString(got, tc.errContains) {
						t.Errorf("error = %q, want it to contain %q", got, tc.errContains)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, cfg)
			}
		})
	}
}

// containsString reports whether s contains substr.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
