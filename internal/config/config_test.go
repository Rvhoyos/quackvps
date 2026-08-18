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
		{"forge above split", func(c *Config) { c.Loader = LoaderForge; c.MCVersion = "1.21" }},
		{"neoforge below split", func(c *Config) { c.MCVersion = "1.20.1" }}, // baseline loader is NeoForge
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
		{"malformed email", func(c *Config) { c.Email = "not-an-email" }},
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

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		in string
		ok bool
	}{
		{"", true},                       // optional
		{"  ", true},                     // blank after trim
		{"you@example.com", true},        // plain address
		{" you@example.com ", true},      // trimmed
		{"a.b+tag@sub.example.co", true}, // still a plain address
		{"not-an-email", false},          // no @
		{"you@localhost", false},         // domain has no dot
		{"You <you@example.com>", false}, // display-name form
		{"a@b@c.com", false},             // two @
	}
	for _, tt := range tests {
		if err := ValidateEmail(tt.in); (err == nil) != tt.ok {
			t.Errorf("ValidateEmail(%q): got err=%v, want ok=%v", tt.in, err, tt.ok)
		}
	}
}

func TestValidateRestore(t *testing.T) {
	// Restore needs a target and a backup, but no loader or MC version.
	base := func() *Config {
		c := New()
		c.Mode = ModeRestore
		c.Parent = "/home/ubuntu/mcserver"
		c.Instance = "survival"
		c.ResolveDir()
		c.RunAsUser = "ubuntu"
		c.Unit = "mc-survival.service"
		c.Backup = "/home/ubuntu/mcserver/survival/backups/world-20260610-161024.zip"
		return c
	}

	if err := base().Validate(); err != nil {
		t.Fatalf("restore config should be valid without a loader/version: %v", err)
	}

	c := base()
	c.Backup = ""
	if err := c.Validate(); err == nil {
		t.Error("expected error when no backup is selected")
	}

	c = base()
	c.Unit = ""
	if err := c.Validate(); err == nil {
		t.Error("expected error when no managing unit is set")
	}
}

func TestValidateLoaderSplitOK(t *testing.T) {
	tests := []struct{ loader, version string }{
		{LoaderForge, "1.20.1"},    // Forge era
		{LoaderNeoForge, "1.21.8"}, // NeoForge era
		{LoaderFabric, "1.20.1"},   // split doesn't apply
		{LoaderFabric, "1.21.8"},
	}
	for _, tt := range tests {
		c := valid()
		c.Loader = tt.loader
		c.MCVersion = tt.version
		if err := c.Validate(); err != nil {
			t.Errorf("%s %s should be valid: %v", tt.loader, tt.version, err)
		}
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
