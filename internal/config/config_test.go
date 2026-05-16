package config_test

import (
	"slices"
	"testing"

	"github.com/furan917/taskwarrior-web-portal/internal/config"
)

func TestAddr_Default(t *testing.T) {
	t.Setenv("TWP_BIND_HOST", "")
	t.Setenv("TWP_BIND_PORT", "")
	if got := config.Addr(); got != "127.0.0.1:5050" {
		t.Errorf("got %q want 127.0.0.1:5050", got)
	}
}

func TestAddr_HostOverride(t *testing.T) {
	t.Setenv("TWP_BIND_HOST", "0.0.0.0")
	t.Setenv("TWP_BIND_PORT", "")
	if got := config.Addr(); got != "0.0.0.0:5050" {
		t.Errorf("got %q want 0.0.0.0:5050", got)
	}
}

func TestAddr_PortOverride(t *testing.T) {
	t.Setenv("TWP_BIND_HOST", "")
	t.Setenv("TWP_BIND_PORT", "8080")
	if got := config.Addr(); got != "127.0.0.1:8080" {
		t.Errorf("got %q want 127.0.0.1:8080", got)
	}
}

func TestAddr_BothOverridden(t *testing.T) {
	t.Setenv("TWP_BIND_HOST", "0.0.0.0")
	t.Setenv("TWP_BIND_PORT", "9000")
	if got := config.Addr(); got != "0.0.0.0:9000" {
		t.Errorf("got %q want 0.0.0.0:9000", got)
	}
}

func TestValidate_ValidPort(t *testing.T) {
	t.Setenv("TWP_BIND_PORT", "8080")
	t.Setenv("TWP_ALLOWED_HOSTS", "")
	if err := config.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_InvalidPort(t *testing.T) {
	for _, bad := range []string{"abc", "0", "65536", "-1"} {
		t.Setenv("TWP_BIND_PORT", bad)
		t.Setenv("TWP_ALLOWED_HOSTS", "")
		if err := config.Validate(); err == nil {
			t.Errorf("TWP_BIND_PORT=%q: expected error", bad)
		}
	}
}

func TestValidate_AllowedHostsBareHostOK(t *testing.T) {
	t.Setenv("TWP_BIND_PORT", "")
	t.Setenv("TWP_ALLOWED_HOSTS", "192.168.1.10")
	if err := config.Validate(); err != nil {
		t.Errorf("bare hostname should be valid, got: %v", err)
	}
}

func TestValidate_AllowedHostsWithPortRejected(t *testing.T) {
	t.Setenv("TWP_BIND_PORT", "")
	t.Setenv("TWP_ALLOWED_HOSTS", "192.168.1.10:5050")
	if err := config.Validate(); err == nil {
		t.Error("expected error for host:port format (port comes from TWP_BIND_PORT)")
	}
}

func TestValidate_Clean(t *testing.T) {
	t.Setenv("TWP_BIND_PORT", "")
	t.Setenv("TWP_ALLOWED_HOSTS", "")
	if err := config.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAllowedHosts_Defaults(t *testing.T) {
	t.Setenv("TWP_BIND_HOST", "")
	t.Setenv("TWP_BIND_PORT", "")
	t.Setenv("TWP_ALLOWED_HOSTS", "")
	got := config.AllowedHosts()
	for _, want := range []string{"localhost:5050", "127.0.0.1:5050"} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %q in %v", want, got)
		}
	}
	if len(got) != 2 {
		t.Errorf("expected 2 hosts, got %v", got)
	}
}

func TestAllowedHosts_ExtraHosts(t *testing.T) {
	t.Setenv("TWP_BIND_HOST", "")
	t.Setenv("TWP_BIND_PORT", "")
	t.Setenv("TWP_ALLOWED_HOSTS", "192.168.1.10, myhostname")
	got := config.AllowedHosts()
	for _, want := range []string{"localhost:5050", "127.0.0.1:5050", "192.168.1.10:5050", "myhostname:5050"} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %q in %v", want, got)
		}
	}
	if len(got) != 4 {
		t.Errorf("expected 4 hosts, got %v", got)
	}
}

func TestAllowedOrigins_Defaults(t *testing.T) {
	t.Setenv("TWP_BIND_HOST", "")
	t.Setenv("TWP_BIND_PORT", "")
	t.Setenv("TWP_ALLOWED_HOSTS", "")
	got := config.AllowedOrigins()
	for _, want := range []string{"http://localhost:5050", "http://127.0.0.1:5050"} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %q in %v", want, got)
		}
	}
	if len(got) != 2 {
		t.Errorf("expected 2 origins, got %v", got)
	}
}

func TestAllowedOrigins_ExtraHosts(t *testing.T) {
	t.Setenv("TWP_BIND_HOST", "")
	t.Setenv("TWP_BIND_PORT", "")
	t.Setenv("TWP_ALLOWED_HOSTS", "192.168.1.10")
	got := config.AllowedOrigins()
	if !slices.Contains(got, "http://192.168.1.10:5050") {
		t.Errorf("missing extra origin in %v", got)
	}
}

func TestSecureCookies_Default(t *testing.T) {
	t.Setenv("TWP_SECURE_COOKIES", "")
	if config.SecureCookies() {
		t.Error("expected false when unset")
	}
}

func TestSecureCookies_One(t *testing.T) {
	t.Setenv("TWP_SECURE_COOKIES", "1")
	if !config.SecureCookies() {
		t.Error("expected true for '1'")
	}
}

func TestSecureCookies_True(t *testing.T) {
	t.Setenv("TWP_SECURE_COOKIES", "true")
	if !config.SecureCookies() {
		t.Error("expected true for 'true'")
	}
}

func TestSecureCookies_Zero(t *testing.T) {
	t.Setenv("TWP_SECURE_COOKIES", "0")
	if config.SecureCookies() {
		t.Error("expected false for '0'")
	}
}

func TestDisableHostCheck_Default(t *testing.T) {
	t.Setenv("TWP_DISABLE_HOST_CHECK", "")
	if config.DisableHostCheck() {
		t.Error("expected false when unset")
	}
}

func TestDisableHostCheck_One(t *testing.T) {
	t.Setenv("TWP_DISABLE_HOST_CHECK", "1")
	if !config.DisableHostCheck() {
		t.Error("expected true for '1'")
	}
}

func TestDisableHostCheck_True(t *testing.T) {
	t.Setenv("TWP_DISABLE_HOST_CHECK", "true")
	if !config.DisableHostCheck() {
		t.Error("expected true for 'true'")
	}
}

func TestDisableHostCheck_Zero(t *testing.T) {
	t.Setenv("TWP_DISABLE_HOST_CHECK", "0")
	if config.DisableHostCheck() {
		t.Error("expected false for '0'")
	}
}
