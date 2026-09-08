package config

import "testing"

func TestLoadRejectsPlaceholderSecrets(t *testing.T) {
	t.Setenv("RVS_ADMIN_TOKEN", "change-this-admin-token")
	t.Setenv("RVS_MASTER_KEY", "change-this-long-random-master-key")
	if _, err := Load(); err == nil {
		t.Fatal("placeholder secrets were accepted")
	}
}

func TestLoadAcceptsExplicitStrongConfiguration(t *testing.T) {
	t.Setenv("RVS_ADMIN_TOKEN", "admin_0123456789abcdefghijklmnop")
	t.Setenv("RVS_MASTER_KEY", "master_0123456789abcdefghijklmnopqrstuvwxyz")
	t.Setenv("RVS_PUBLIC_URL", "https://storage.example.test/")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.PublicURL != "https://storage.example.test" {
		t.Fatalf("normalized public URL = %q", c.PublicURL)
	}
}
