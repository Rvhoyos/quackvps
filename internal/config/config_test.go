package config

import "testing"

// valid returns a Config that passes Validate; tests mutate a copy to check one
// failure at a time.
func valid() *Config {
	c := New()
	c.Mode = ModeInstall
	c.Parent = "/home/ubuntu/mcserver"
	c.Instance = "survival"
	c.ResolveDir()
	c.Loader = LoaderNeoForge
	c.MCVersion = "1.21.8"
	c.ServerPort = 25565
	c.RunAsUser = "ubuntu"
	c.RunAsHome = "/home/ubuntu"
	return c
}

func TestValidateOK(t *testing.T) {
	if err := valid().Validate(); err != nil {
		t.Fatalf("baseline config should be valid: %v", err)
	}
}

func TestValidateFailures(t *testing.T) {
	tests := []struct {
		name   string
		break_ func(*Config)
	}{
		{"relative parent", func(c *Config) { c.Parent = "mcserver"; c.ResolveDir() }},
		{"instance with sep", func(c *Config) { c.Instance = "a/b"; c.ResolveDir() }},
		{"dir not resolved", func(c *Config) { c.Dir = "/wrong" }},
		{"unknown loader", func(c *Config) { c.Loader = "paper" }},
		{"mc too old", func(c *Config) { c.MCVersion = "1.16.5" }},
		{"mc snapshot", func(c *Config) { c.MCVersion = "24w14a" }},
		{"zero xms", func(c *Config) { c.HeapMinGB = 0 }},
		{"xmx below xms", func(c *Config) { c.HeapMinGB = 6; c.HeapMaxGB = 4 }},
		{"bad server port", func(c *Config) { c.ServerPort = 70000 }},
		{"root run-as", func(c *Config) { c.RunAsUser = "root" }},
		{"feature without port", func(c *Config) { c.BlueMap = true }},
		{"web feature without subdomain", func(c *Config) {
			c.BlueMap = true
			c.Ports[PortBlueMap] = 8100
			c.Domain = "example.com"
		}},
		{"colliding ports", func(c *Config) {
			c.Votifier = true
			c.Ports[PortVotifier] = 25565 // same as server port
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := valid()
			tt.break_(c)
			if err := c.Validate(); err == nil {
				t.Errorf("expected validation error for %s", tt.name)
			}
		})
	}
}

func TestValidateWebFeatureNoDomainOK(t *testing.T) {
	c := valid()
	c.BlueMap = true
	c.Ports[PortBlueMap] = 8100
	// No domain → no subdomain required (localhost + ssh -L path).
	if err := c.Validate(); err != nil {
		t.Fatalf("web feature without domain should be valid: %v", err)
	}
}
