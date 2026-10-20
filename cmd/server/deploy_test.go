package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

// Settings an operator puts in .env must reach the server (empty keeps the server default),
// and the port is published on loopback unless KYPULSE_BIND says otherwise.
func TestComposeForwardsSettingsAndBindsLoopback(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, name := range []string{"KYPULSE_ADMIN_PASSWORD", "KYPULSE_TRUSTED_PROXIES", "KYPULSE_KYSIGNON_ISSUER", "KYPULSE_KYSIGNON_CLIENT_ID", "KYPULSE_KYSIGNON_SECRET", "KYPULSE_KYSIGNON_HMAC_SECRET"} {
		if !regexp.MustCompile(`(?m)^\s+- ` + name + `=\$\{` + name + `:-\}$`).MatchString(body) {
			t.Errorf("docker-compose.yml does not forward %s", name)
		}
	}
	ports := regexp.MustCompile(`(?m)^\s+- "(\S+)"$`).FindAllStringSubmatch(body[strings.Index(body, "ports:"):], 1)
	if len(ports) != 1 || ports[0][1] != "${KYPULSE_BIND:-127.0.0.1}:8080:8080" {
		t.Errorf("app port is not loopback by default: %v", ports)
	}
}
