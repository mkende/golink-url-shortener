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

// minimalValid is a base TOML that satisfies all required fields.
// When appending additional top-level keys, insert them before anonSection.
const (
	minimalHeader = `canonical_address = "https://go.example.com"` + "\n"
	anonSection   = "[anonymous]\nenabled = true\n"
	minimalValid  = minimalHeader + anonSection
)

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
			toml: minimalValid,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				if cfg.CanonicalAddress != "https://go.example.com" {
					t.Errorf("CanonicalAddress = %q, want %q", cfg.CanonicalAddress, "https://go.example.com")
				}
				if cfg.CanonicalHost() != "go.example.com" {
					t.Errorf("CanonicalHost() = %q, want %q", cfg.CanonicalHost(), "go.example.com")
				}
				if cfg.CanonicalScheme() != "https" {
					t.Errorf("CanonicalScheme() = %q, want %q", cfg.CanonicalScheme(), "https")
				}
			},
		},
		{
			name: "http scheme in canonical_address is valid",
			toml: `
canonical_address = "http://go"
[anonymous]
enabled = true
`,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				if cfg.CanonicalScheme() != "http" {
					t.Errorf("CanonicalScheme() = %q, want %q", cfg.CanonicalScheme(), "http")
				}
				if cfg.CanonicalHost() != "go" {
					t.Errorf("CanonicalHost() = %q, want %q", cfg.CanonicalHost(), "go")
				}
			},
		},
		{
			name: "canonical_address optional when oidc disabled",
			toml: `
[anonymous]
enabled = true
`,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				if cfg.CanonicalAddress != "" {
					t.Errorf("CanonicalAddress = %q, want empty", cfg.CanonicalAddress)
				}
				if cfg.CanonicalHost() != "" {
					t.Errorf("CanonicalHost() = %q, want empty", cfg.CanonicalHost())
				}
			},
		},
		{
			name: "defaults applied when fields omitted",
			toml: minimalValid,
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
canonical_address = "https://go.corp.example.com"
listen_addr       = "127.0.0.1:9090"
title             = "Corp Links"
quick_link_length = 8

[anonymous]
enabled = true

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
			name:        "canonical_address required when oidc enabled",
			toml:        "jwt_secret = \"secret\"\n[oidc]\nenabled = true",
			wantErr:     true,
			errContains: "canonical_address is required when oidc is enabled",
		},
		{
			name:        "invalid canonical_address scheme returns error",
			toml:        "canonical_address = \"ftp://go.example.com\"\n[anonymous]\nenabled = true",
			wantErr:     true,
			errContains: "canonical_address scheme must be http or https",
		},
		{
			name:        "canonical_address without scheme returns error",
			toml:        "canonical_address = \"go.example.com\"\n[anonymous]\nenabled = true",
			wantErr:     true,
			errContains: "canonical_address must be a valid URL",
		},
		{
			name:        "no auth provider returns error",
			toml:        minimalHeader, // no auth section
			wantErr:     true,
			errContains: "at least one authentication provider",
		},
		{
			name:        "bad TOML syntax returns error",
			toml:        `canonical_address = [[[`,
			wantErr:     true,
			errContains: "parsing config file",
		},
		{
			name: "quick_link_length exactly 4 is valid",
			toml: minimalHeader + "quick_link_length = 4\n" + anonSection,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				if cfg.QuickLinkLength != 4 {
					t.Errorf("QuickLinkLength = %d, want 4", cfg.QuickLinkLength)
				}
			},
		},
		{
			name:        "quick_link_length less than 4 returns error",
			toml:        minimalHeader + "quick_link_length = 3\n" + anonSection,
			wantErr:     true,
			errContains: "quick_link_length must be >= 4",
		},
		{
			name:        "invalid db driver returns error",
			toml:        minimalValid + "[db]\ndriver = \"mysql\"\n",
			wantErr:     true,
			errContains: "db.driver must be",
		},
		{
			name: "admin fields parsed correctly",
			toml: minimalHeader + `admin_emails = ["alice@example.com", "bob@example.com"]` + "\n" +
				`admin_groups = ["sre", "ops"]` + "\n" + anonSection,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				if len(cfg.AdminEmails) != 2 {
					t.Errorf("len(AdminEmails) = %d, want 2", len(cfg.AdminEmails))
				}
				if len(cfg.AdminGroups) != 2 || cfg.AdminGroups[0] != "sre" {
					t.Errorf("AdminGroups = %v, want [sre ops]", cfg.AdminGroups)
				}
			},
		},
		{
			name: "trusted_proxy parsed and validated",
			toml: minimalHeader + `trusted_proxy = ["127.0.0.0/8", "10.0.0.0/8"]` + "\n" + anonSection,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				if len(cfg.TrustedProxy) != 2 {
					t.Errorf("len(TrustedProxy) = %d, want 2", len(cfg.TrustedProxy))
				}
			},
		},
		{
			name:        "invalid trusted_proxy CIDR returns error",
			toml:        minimalHeader + `trusted_proxy = ["not-a-cidr"]` + "\n" + anonSection,
			wantErr:     true,
			errContains: "trusted_proxy",
		},
		{
			name:        "tailscale without trusted_proxy returns error",
			toml:        minimalValid + "[tailscale]\nenabled = true\n",
			wantErr:     true,
			errContains: "trusted_proxy must be set when tailscale auth is enabled",
		},
		{
			name:        "proxy_auth without trusted_proxy returns error",
			toml:        minimalValid + "[proxy_auth]\nenabled = true\n", // proxy_auth needs trusted_proxy
			wantErr:     true,
			errContains: "trusted_proxy must be set when proxy_auth is enabled",
		},
		{
			name:        "unknown top-level key returns error",
			toml:        minimalValid + "typo_key = true\n",
			wantErr:     true,
			errContains: "unknown configuration key(s)",
		},
		{
			name:        "unknown nested key returns error",
			toml:        minimalValid + "[oidc]\ntypo_field = true\n",
			wantErr:     true,
			errContains: "unknown configuration key(s)",
		},
		{
			name:        "jwt_secret_env_var referencing unset variable returns error",
			toml:        minimalHeader + "jwt_secret_env_var = \"GOLINK_UNSET_VAR_12345\"\n" + anonSection,
			wantErr:     true,
			errContains: "jwt_secret_env_var",
		},
		{
			name:        "client_secret_env_var referencing unset variable returns error",
			toml:        minimalHeader + "jwt_secret = \"s\"\n[oidc]\nenabled = true\nclient_id = \"id\"\nclient_secret_env_var = \"GOLINK_UNSET_VAR_12345\"\n",
			wantErr:     true,
			errContains: "client_secret_env_var",
		},
		{
			name:        "non-existent file returns error",
			toml:        "", // not used — we pass a bad path instead
			wantErr:     true,
			errContains: "reading config file",
		},
		// allow_advanced_links and domains_for_advanced_links
		{
			name: "allow_advanced_links defaults to true when omitted",
			toml: minimalValid,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				if !cfg.AdvancedLinksAllowed() {
					t.Error("AdvancedLinksAllowed() = false, want true (default)")
				}
			},
		},
		{
			name: "allow_advanced_links = true is explicit",
			toml: minimalHeader + "allow_advanced_links = true\n" + anonSection,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				if !cfg.AdvancedLinksAllowed() {
					t.Error("AdvancedLinksAllowed() = false, want true")
				}
			},
		},
		{
			name: "allow_advanced_links = false disables advanced links",
			toml: minimalHeader + "allow_advanced_links = false\n" + anonSection,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				if cfg.AdvancedLinksAllowed() {
					t.Error("AdvancedLinksAllowed() = true, want false")
				}
			},
		},
		{
			name: "valid domains_for_advanced_links",
			toml: minimalHeader + `domains_for_advanced_links = ["example.com", "*.internal.corp"]` + "\n" + anonSection,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				if len(cfg.DomainsForAdvancedLinks) != 2 {
					t.Errorf("len(DomainsForAdvancedLinks) = %d, want 2", len(cfg.DomainsForAdvancedLinks))
				}
			},
		},
		{
			name:        "invalid domain pattern in domains_for_advanced_links",
			toml:        minimalHeader + `domains_for_advanced_links = ["exam_ple.com"]` + "\n" + anonSection,
			wantErr:     true,
			errContains: "domains_for_advanced_links",
		},
		{
			name:        "wildcard without base domain rejected",
			toml:        minimalHeader + `domains_for_advanced_links = ["*."]` + "\n" + anonSection,
			wantErr:     true,
			errContains: "domains_for_advanced_links",
		},
		{
			name:        "star-only pattern rejected",
			toml:        minimalHeader + `domains_for_advanced_links = ["*"]` + "\n" + anonSection,
			wantErr:     true,
			errContains: "domains_for_advanced_links",
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

// TestLoadEnvVarSecrets tests secret resolution via _env_var config fields. It
// is not marked parallel because subtests use t.Setenv, which requires a
// non-parallel parent.
func TestLoadEnvVarSecrets(t *testing.T) {
	tests := []struct {
		name        string
		toml        string
		setup       func(t *testing.T)
		wantErr     bool
		errContains string
		check       func(t *testing.T, cfg *config.Config)
	}{
		{
			name:  "jwt_secret_env_var populates JWTSecret",
			toml:  minimalHeader + "jwt_secret_env_var = \"TEST_JWT_SECRET\"\n" + anonSection,
			setup: func(t *testing.T) { t.Helper(); t.Setenv("TEST_JWT_SECRET", "from-env") },
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				if cfg.JWTSecret != "from-env" {
					t.Errorf("JWTSecret = %q, want %q", cfg.JWTSecret, "from-env")
				}
			},
		},
		{
			name:        "both jwt_secret and jwt_secret_env_var returns error",
			toml:        minimalHeader + "jwt_secret = \"inline\"\njwt_secret_env_var = \"TEST_JWT_SECRET\"\n" + anonSection,
			setup:       func(t *testing.T) { t.Helper(); t.Setenv("TEST_JWT_SECRET", "from-env") },
			wantErr:     true,
			errContains: "cannot set both jwt_secret and jwt_secret_env_var",
		},
		{
			name:  "oidc client_secret_env_var populates ClientSecret",
			toml:  minimalHeader + "jwt_secret = \"s\"\n[oidc]\nenabled = true\nclient_id = \"id\"\nclient_secret_env_var = \"TEST_OIDC_SECRET\"\n",
			setup: func(t *testing.T) { t.Helper(); t.Setenv("TEST_OIDC_SECRET", "oidc-from-env") },
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				if cfg.OIDC.ClientSecret != "oidc-from-env" {
					t.Errorf("OIDC.ClientSecret = %q, want %q", cfg.OIDC.ClientSecret, "oidc-from-env")
				}
			},
		},
		{
			name:        "both client_secret and client_secret_env_var returns error",
			toml:        minimalHeader + "jwt_secret = \"s\"\n[oidc]\nenabled = true\nclient_id = \"id\"\nclient_secret = \"inline\"\nclient_secret_env_var = \"TEST_OIDC_SECRET\"\n",
			setup:       func(t *testing.T) { t.Helper(); t.Setenv("TEST_OIDC_SECRET", "oidc-from-env") },
			wantErr:     true,
			errContains: "cannot set both oidc.client_secret and oidc.client_secret_env_var",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setup != nil {
				tc.setup(t)
			}

			path := writeTemp(t, tc.toml)
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
