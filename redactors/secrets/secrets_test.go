package secrets

import (
	"regexp"
	"strings"
	"sync"
	"testing"
)

func TestNew(t *testing.T) {
	r := New()
	if r == nil {
		t.Fatal("New() should not return nil")
	}
	if len(r.patterns) == 0 {
		t.Error("New() should have default patterns")
	}
}

func TestNewWithPatterns(t *testing.T) {
	custom := []Pattern{
		{Name: "test", Regex: regexp.MustCompile(`secret_\d+`), Replace: "[REDACTED]"},
	}
	r := NewWithPatterns(custom)
	if len(r.patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(r.patterns))
	}
	if r.patterns[0].Name != "test" {
		t.Errorf("pattern name = %q, want %q", r.patterns[0].Name, "test")
	}
}

func TestNewWithPatterns_Empty(t *testing.T) {
	r := NewWithPatterns(nil)
	if got := r.Redact("anything"); got != "anything" {
		t.Errorf("no patterns should pass through, got %q", got)
	}
}

func TestRedact(t *testing.T) {
	r := New()

	tests := []struct {
		name     string
		input    string
		contains string // expected replacement marker
	}{
		{
			name:     "AWS access key",
			input:    "key is AKIAIOSFODNN7EXAMPLE",
			contains: "[REDACTED_AWS_KEY]",
		},
		{
			name:     "AWS secret key",
			input:    "aws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY1",
			contains: "[REDACTED_AWS_SECRET]",
		},
		{
			name:     "GitHub personal access token",
			input:    "token: ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijkl",
			contains: "[REDACTED_GITHUB_TOKEN]",
		},
		{
			name:     "GitHub fine-grained PAT",
			input:    "pat: github_pat_ABCDEFGHIJKLMNOPQRSTUV1234",
			contains: "[REDACTED_GITHUB_PAT]",
		},
		{
			name:     "GitLab token",
			input:    "token: glpat-ABCDEFGHIJKLMNOPQRST",
			contains: "[REDACTED_GITLAB_TOKEN]",
		},
		{
			name:     "Slack bot token",
			input:    "token: xoxb-1234567890-abcdefghij",
			contains: "[REDACTED_SLACK_TOKEN]",
		},
		{
			name:     "Stripe secret key",
			input:    "key: " + "sk_" + "live_ABCDEFGHIJKLMNOPQRSTUVWX",
			contains: "[REDACTED_STRIPE_KEY]",
		},
		{
			name:     "PostgreSQL connection string",
			input:    "postgres://admin:s3cretP4ss@db.example.com:5432/mydb",
			contains: "[REDACTED_CONNECTION_STRING]",
		},
		{
			name:     "MongoDB connection string",
			input:    "mongodb://user:password@host:27017/db",
			contains: "[REDACTED_CONNECTION_STRING]",
		},
		{
			name:     "Redis connection string",
			input:    "redis://default:mypassword@redis.example.com:6379/0",
			contains: "[REDACTED_CONNECTION_STRING]",
		},
		{
			name:     "JWT token",
			input:    "token eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0.abc123def456",
			contains: "[REDACTED_JWT]",
		},
		{
			name:     "generic password",
			input:    `password="SuperS3cretPassw0rd!"`,
			contains: "[REDACTED_PASSWORD]",
		},
		{
			name:     "generic API key",
			input:    "api_key=abcdefghijklmnopqrstuvwxyz",
			contains: "[REDACTED_API_KEY]",
		},
		{
			name:     "bearer token",
			input:    "Authorization: Bearer eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiJ9",
			contains: "[REDACTED_BEARER_TOKEN]",
		},
		{
			name:     "private key header RSA",
			input:    "-----BEGIN RSA PRIVATE KEY-----",
			contains: "[REDACTED_PRIVATE_KEY]",
		},
		{
			name:     "private key header generic",
			input:    "-----BEGIN PRIVATE KEY-----",
			contains: "[REDACTED_PRIVATE_KEY]",
		},
		{
			name:     "export secret env var",
			input:    "export GITHUB_TOKEN=ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijkl",
			contains: "[REDACTED",
		},
		{
			name:     "NPM token",
			input:    "npm_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij",
			contains: "[REDACTED_NPM_TOKEN]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.Redact(tt.input)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("Redact(%q) = %q, want to contain %q", tt.input, got, tt.contains)
			}
			// the original secret should not appear verbatim
			if got == tt.input {
				t.Errorf("Redact should have modified the input, got unchanged: %q", got)
			}
		})
	}
}

func TestRedact_EmptyString(t *testing.T) {
	r := New()
	if got := r.Redact(""); got != "" {
		t.Errorf("Redact empty = %q, want empty", got)
	}
}

func TestRedact_NoSecrets(t *testing.T) {
	r := New()
	clean := "mycli agent prime --verbose --format=json"
	if got := r.Redact(clean); got != clean {
		t.Errorf("Redact should not modify clean input, got %q", got)
	}
}

func TestRedact_MultipleSecrets(t *testing.T) {
	r := New()
	input := "AKIAIOSFODNN7EXAMPLE postgres://admin:pass@host/db"
	got := r.Redact(input)
	if !strings.Contains(got, "[REDACTED_AWS_KEY]") {
		t.Error("should redact AWS key")
	}
	if !strings.Contains(got, "[REDACTED_CONNECTION_STRING]") {
		t.Error("should redact connection string")
	}
}

func TestContainsSecrets(t *testing.T) {
	r := New()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "has AWS key", input: "AKIAIOSFODNN7EXAMPLE", want: true},
		{name: "has GitHub token", input: "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijkl", want: true},
		{name: "has connection string", input: "postgres://user:pass@host/db", want: true},
		{name: "clean string", input: "mycli status --verbose", want: false},
		{name: "empty string", input: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.ContainsSecrets(tt.input); got != tt.want {
				t.Errorf("ContainsSecrets(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestAddPattern(t *testing.T) {
	r := New()
	originalCount := len(r.patterns)

	custom := Pattern{
		Name:    "custom_secret",
		Regex:   regexp.MustCompile(`CUSTOM_[A-Z]{10}`),
		Replace: "[REDACTED_CUSTOM]",
	}
	r.AddPattern(custom)

	if len(r.patterns) != originalCount+1 {
		t.Errorf("pattern count = %d, want %d", len(r.patterns), originalCount+1)
	}

	// the new pattern should work
	input := "key: CUSTOM_ABCDEFGHIJ"
	got := r.Redact(input)
	if !strings.Contains(got, "[REDACTED_CUSTOM]") {
		t.Errorf("custom pattern not applied, got %q", got)
	}
}

func TestAddPattern_DoesNotAffectExisting(t *testing.T) {
	r := New()

	// existing patterns should still work after adding a new one
	r.AddPattern(Pattern{
		Name:    "noop",
		Regex:   regexp.MustCompile(`NOOP_MATCH`),
		Replace: "[NOOP]",
	})

	input := "AKIAIOSFODNN7EXAMPLE"
	got := r.Redact(input)
	if !strings.Contains(got, "[REDACTED_AWS_KEY]") {
		t.Error("existing patterns should still work after AddPattern")
	}
}

func TestRedactor_NilRegexPattern(t *testing.T) {
	// pattern with nil regex should not cause panic
	r := NewWithPatterns([]Pattern{
		{Name: "nil_regex", Regex: nil, Replace: "[X]"},
		{Name: "valid", Regex: regexp.MustCompile(`secret`), Replace: "[Y]"},
	})
	got := r.Redact("my secret value")
	if !strings.Contains(got, "[Y]") {
		t.Errorf("valid pattern should still work, got %q", got)
	}
}

func TestRedactor_ConcurrentAccess(t *testing.T) {
	r := New()
	var wg sync.WaitGroup

	// concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Redact("AKIAIOSFODNN7EXAMPLE some text")
			r.ContainsSecrets("postgres://user:pass@host/db")
		}()
	}

	// concurrent write
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.AddPattern(Pattern{
			Name:    "concurrent",
			Regex:   regexp.MustCompile(`CONCURRENT`),
			Replace: "[C]",
		})
	}()

	wg.Wait()
}

func TestDefaultPatterns_Coverage(t *testing.T) {
	patterns := DefaultPatterns()
	if len(patterns) == 0 {
		t.Fatal("DefaultPatterns should return non-empty list")
	}

	// verify all patterns have required fields
	for i, p := range patterns {
		if p.Name == "" {
			t.Errorf("pattern[%d] has empty name", i)
		}
		if p.Regex == nil {
			t.Errorf("pattern[%d] (%s) has nil regex", i, p.Name)
		}
		if p.Replace == "" {
			t.Errorf("pattern[%d] (%s) has empty replacement", i, p.Name)
		}
	}
}

func TestRedact_Twilio(t *testing.T) {
	r := New()
	input := "SK" + "0123456789abcdef0123456789abcdef"
	got := r.Redact(input)
	if !strings.Contains(got, "[REDACTED_TWILIO_KEY]") {
		t.Errorf("should redact Twilio key, got %q", got)
	}
}

func TestRedact_SendGrid(t *testing.T) {
	r := New()
	input := "SG.abcdefghijklmnopqrstuv.ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqr"
	got := r.Redact(input)
	if !strings.Contains(got, "[REDACTED_SENDGRID_KEY]") {
		t.Errorf("should redact SendGrid key, got %q", got)
	}
}

func TestRedact_BasicAuth(t *testing.T) {
	r := New()
	input := "authorization: basic dXNlcjpwYXNzd29yZA=="
	got := r.Redact(input)
	if !strings.Contains(got, "[REDACTED_BASIC_AUTH]") {
		t.Errorf("should redact basic auth, got %q", got)
	}
}

func TestContainsSecrets_DoesNotModifyInput(t *testing.T) {
	r := New()
	input := "AKIAIOSFODNN7EXAMPLE"
	_ = r.ContainsSecrets(input)
	// input should not be modified (it shouldn't be, but verify interface contract)
	if input != "AKIAIOSFODNN7EXAMPLE" {
		t.Error("ContainsSecrets should not modify input")
	}
}
