package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// The shipped deployment files carry the defaults the server relies on; CI's docker job proves
// the image itself runs that way. These hold the files to it.
func TestDeploymentFilesStayHardened(t *testing.T) {
	read := func(name string) string {
		raw, err := os.ReadFile(filepath.Join("..", "..", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	for file, rules := range map[string][]string{
		"docker-compose.yml": {
			`(?m)^\s+read_only: true$`,
			`(?m)^\s+cap_drop: \[ALL\]$`,
			`(?m)^\s+- no-new-privileges:true$`,
			`(?m)^\s+- KYPULSE_ENV=production$`,
			`(?m)^\s+- KYPULSE_AUDIT_KEY_FILE=/app/secrets/audit\.key$`,
			`(?m)^\s+- \./secrets:/app/secrets$`,
			`(?m)^\s+POSTGRES_PASSWORD: \$\{POSTGRES_PASSWORD:-\}$`,
		},
		"Dockerfile": {
			`(?m)^USER 10001:10001$`,
			`(?m)^HEALTHCHECK .*$`,
			`(?m)^RUN npm ci$`,
			`(?m)^ENV KYPULSE_ENV=production$`,
		},
	} {
		body := read(file)
		for _, rule := range rules {
			if !regexp.MustCompile(rule).MatchString(body) {
				t.Errorf("%s lacks %s", file, rule)
			}
		}
	}
	if regexp.MustCompile(`(?m)^\s+- "?(\S+:)?5432:5432"?$`).MatchString(read("docker-compose.yml")) {
		t.Error("docker-compose.yml publishes PostgreSQL")
	}
}
