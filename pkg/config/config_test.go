package config

import "testing"

func TestFixtureModeCannotEscapeLoopback(t *testing.T) {
	base := Config{Address: "127.0.0.1:8080", PublicURL: "http://127.0.0.1:8080", DatabaseURL: "postgres://db", Development: true, FixtureAuth: true, Edition: "self-hosted", EncryptionKey: "dGVzdHRlc3R0ZXN0dGVzdHRlc3R0ZXN0dGVzdHRlc3Q="}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Config){
		func(c *Config) { c.Development = false },
		func(c *Config) { c.Address = "0.0.0.0:8080" },
		func(c *Config) { c.PublicURL = "http://localhost.evil.test" },
		func(c *Config) { c.Edition = "hosted" },
		func(c *Config) { c.PublicURL = "http://user@127.0.0.1:8080" },
	} {
		c := base
		mutate(&c)
		if c.Validate() == nil {
			t.Fatalf("accepted unsafe configuration: %+v", c)
		}
	}
}
