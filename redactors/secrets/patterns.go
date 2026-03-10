package secrets

import "regexp"

// DefaultPatterns returns built-in secret patterns covering common credential types.
// Patterns are ordered roughly by specificity (more specific patterns first).
func DefaultPatterns() []Pattern {
	return []Pattern{
		// AWS Access Keys (AKIA... format, exactly 20 chars)
		{
			Name:    "aws_access_key",
			Regex:   regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
			Replace: "[REDACTED_AWS_KEY]",
		},

		// AWS Secret Keys (40 char base64, usually after key= or similar)
		{
			Name:    "aws_secret_key",
			Regex:   regexp.MustCompile(`(?i)(aws_secret_access_key|aws_secret_key|secret_access_key)\s*[=:]\s*['"]?([A-Za-z0-9/+=]{40})['"]?`),
			Replace: "[REDACTED_AWS_SECRET]",
		},

		// GitHub tokens (ghp_, gho_, ghs_, ghr_, ghu_ prefixes)
		{
			Name:    "github_token",
			Regex:   regexp.MustCompile(`gh[psortu]_[A-Za-z0-9_]{36,255}`),
			Replace: "[REDACTED_GITHUB_TOKEN]",
		},

		// GitHub fine-grained PAT (github_pat_ prefix)
		{
			Name:    "github_fine_grained_pat",
			Regex:   regexp.MustCompile(`github_pat_[A-Za-z0-9_]{22,255}`),
			Replace: "[REDACTED_GITHUB_PAT]",
		},

		// GitLab tokens (glpat- prefix)
		{
			Name:    "gitlab_token",
			Regex:   regexp.MustCompile(`glpat-[A-Za-z0-9\-_]{20,}`),
			Replace: "[REDACTED_GITLAB_TOKEN]",
		},

		// Slack tokens (xoxb-, xoxp-, xoxa-, xoxs-, xoxr-)
		{
			Name:    "slack_token",
			Regex:   regexp.MustCompile(`xox[abpsr]-[A-Za-z0-9\-]{10,}`),
			Replace: "[REDACTED_SLACK_TOKEN]",
		},

		// Stripe API keys (sk_live_, sk_test_, pk_live_, pk_test_)
		{
			Name:    "stripe_key",
			Regex:   regexp.MustCompile(`[sr]k_(live|test)_[A-Za-z0-9]{24,}`),
			Replace: "[REDACTED_STRIPE_KEY]",
		},

		// Twilio API keys and auth tokens
		{
			Name:    "twilio_key",
			Regex:   regexp.MustCompile(`SK[a-f0-9]{32}`),
			Replace: "[REDACTED_TWILIO_KEY]",
		},

		// SendGrid API keys
		{
			Name:    "sendgrid_key",
			Regex:   regexp.MustCompile(`SG\.[A-Za-z0-9_\-]{22}\.[A-Za-z0-9_\-]{43}`),
			Replace: "[REDACTED_SENDGRID_KEY]",
		},

		// Mailchimp API keys
		{
			Name:    "mailchimp_key",
			Regex:   regexp.MustCompile(`[a-f0-9]{32}-us[0-9]{1,2}`),
			Replace: "[REDACTED_MAILCHIMP_KEY]",
		},

		// NPM tokens
		{
			Name:    "npm_token",
			Regex:   regexp.MustCompile(`npm_[A-Za-z0-9]{36}`),
			Replace: "[REDACTED_NPM_TOKEN]",
		},

		// PyPI tokens
		{
			Name:    "pypi_token",
			Regex:   regexp.MustCompile(`pypi-[A-Za-z0-9_\-]{50,}`),
			Replace: "[REDACTED_PYPI_TOKEN]",
		},

		// Heroku API keys (UUIDs)
		{
			Name:    "heroku_key",
			Regex:   regexp.MustCompile(`[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`),
			Replace: "[REDACTED_UUID]",
		},

		// Private keys (RSA, DSA, EC, OPENSSH)
		{
			Name:    "private_key_header",
			Regex:   regexp.MustCompile(`-----BEGIN\s+(RSA|DSA|EC|OPENSSH|PGP)?\s*PRIVATE KEY-----`),
			Replace: "[REDACTED_PRIVATE_KEY]",
		},

		// Generic private key (fallback)
		{
			Name:    "private_key_generic",
			Regex:   regexp.MustCompile(`-----BEGIN PRIVATE KEY-----`),
			Replace: "[REDACTED_PRIVATE_KEY]",
		},

		// Base64-encoded secrets in environment variables
		{
			Name:    "export_aws_secret",
			Regex:   regexp.MustCompile(`(?i)export\s+(AWS_SECRET_ACCESS_KEY|AWS_SESSION_TOKEN)\s*=\s*['"]?[A-Za-z0-9/+=]{20,}['"]?`),
			Replace: "[REDACTED_EXPORT]",
		},

		// Generic export of sensitive env vars
		{
			Name:    "export_secret",
			Regex:   regexp.MustCompile(`(?i)export\s+(GITHUB_TOKEN|GITLAB_TOKEN|API_KEY|SECRET_KEY|AUTH_TOKEN|ACCESS_TOKEN|PRIVATE_KEY|PASSWORD|PASSWD|DB_PASSWORD|DATABASE_PASSWORD|MYSQL_PASSWORD|POSTGRES_PASSWORD|REDIS_PASSWORD|MONGO_PASSWORD)\s*=\s*['"]?[^'"\s]+['"]?`),
			Replace: "[REDACTED_EXPORT]",
		},

		// Connection strings with embedded credentials
		{
			Name:    "connection_string",
			Regex:   regexp.MustCompile(`(?i)(mongodb|postgres|postgresql|mysql|redis|amqp|mssql):\/\/[^:]+:[^@]+@[^\s'"]+`),
			Replace: "[REDACTED_CONNECTION_STRING]",
		},

		// Bearer tokens in headers
		{
			Name:    "bearer_token",
			Regex:   regexp.MustCompile(`(?i)(authorization|bearer)\s*[:=]\s*['"]?bearer\s+[A-Za-z0-9_\-\.]{20,}['"]?`),
			Replace: "[REDACTED_BEARER_TOKEN]",
		},

		// Basic auth headers (base64)
		{
			Name:    "basic_auth",
			Regex:   regexp.MustCompile(`(?i)authorization\s*[:=]\s*['"]?basic\s+[A-Za-z0-9+/=]{10,}['"]?`),
			Replace: "[REDACTED_BASIC_AUTH]",
		},

		// Generic API key patterns (must be after more specific patterns)
		{
			Name:    "generic_api_key",
			Regex:   regexp.MustCompile(`(?i)(api[_-]?key|apikey)\s*[=:]\s*['"]?([A-Za-z0-9_\-]{20,})['"]?`),
			Replace: "[REDACTED_API_KEY]",
		},

		// Generic token patterns
		{
			Name:    "generic_token",
			Regex:   regexp.MustCompile(`(?i)(access[_-]?token|auth[_-]?token|secret[_-]?token)\s*[=:]\s*['"]?([A-Za-z0-9_\-]{20,})['"]?`),
			Replace: "[REDACTED_TOKEN]",
		},

		// Generic password patterns
		{
			Name:    "generic_password",
			Regex:   regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[=:]\s*['"]([^'"]{8,})['"]`),
			Replace: "[REDACTED_PASSWORD]",
		},

		// Generic secret patterns
		{
			Name:    "generic_secret",
			Regex:   regexp.MustCompile(`(?i)(secret|secret_key|client_secret)\s*[=:]\s*['"]?([A-Za-z0-9_\-/+=]{16,})['"]?`),
			Replace: "[REDACTED_SECRET]",
		},

		// JWT tokens (header.payload.signature format)
		{
			Name:    "jwt_token",
			Regex:   regexp.MustCompile(`eyJ[A-Za-z0-9_-]*\.eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*`),
			Replace: "[REDACTED_JWT]",
		},
	}
}
