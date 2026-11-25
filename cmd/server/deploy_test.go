package main

import (
	"crypto/sha256"
	"fmt"
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

// The bundled PostgreSQL is 18, pinned by digest, on the volume layout 18 accepts, and an
// install that ran 17 is moved by the vendored, unmodified upgrade script: never a bare image
// bump, which leaves 18 refusing the old mount. scripts/test-postgres-17-to-18.sh runs it.
func TestComposePostgres18UpgradesFrom17(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	service := func(name string) string {
		m := regexp.MustCompile(`(?ms)^  ` + name + `:\n(.*?)(?:\n\n|\z)`).FindStringSubmatch(body)
		if m == nil {
			t.Fatalf("docker-compose.yml has no %s service", name)
		}
		return m[1]
	}
	for name, rules := range map[string][]string{
		"postgres": {
			`(?m)^    image: postgres:18\.7-alpine3\.24@sha256:0115267ab5a509022df9319c77be365cef335adb71556c75f60f5360ee247e71$`,
			`(?m)^    entrypoint: \[bash, /ky/postgres-17-to-18\.sh, restore\]$`,
			`(?m)^    command: \[postgres\]$`,
			`(?m)^    stop_grace_period: 2m$`,
			`(?m)^      postgres-17-dump:\n        condition: service_completed_successfully$`,
			`(?m)^      - postgres:/var/lib/postgresql$`,
			`(?m)^      - \./scripts/postgres-17-to-18\.sh:/ky/postgres-17-to-18\.sh:ro$`,
		},
		"postgres-17-dump": {
			`(?m)^    image: postgres:17\.12-alpine3\.24@sha256:f9cf3e1f0066f0eb0c48707d864f0ac5941e9ff454a072077d97a4d278d07d73$`,
			`(?m)^    network_mode: none$`,
			`(?m)^    stop_grace_period: 2m$`,
			`(?m)^    restart: "no"$`,
			`(?m)^    profiles: \["postgres"\]$`,
			`(?m)^    entrypoint: \[bash, /ky/postgres-17-to-18\.sh, dump\]$`,
			`(?m)^      POSTGRES_USER: postgres$`,
			`(?m)^      OLD: /var/lib/postgresql/data$`,
			`(?m)^      NEW: /pg18$`,
			`(?m)^      - pgdata:/var/lib/postgresql/data$`,
			`(?m)^      - postgres:/pg18$`,
			`(?m)^      - \./scripts/postgres-17-to-18\.sh:/ky/postgres-17-to-18\.sh:ro$`,
		},
	} {
		s := service(name)
		for _, rule := range rules {
			if !regexp.MustCompile(rule).MatchString(s) {
				t.Errorf("%s service lacks %s", name, rule)
			}
		}
	}
	if strings.Contains(service("postgres"), "/var/lib/postgresql/data") || strings.Contains(service("postgres"), "PGDATA") {
		t.Error("the 18 service mounts the 17 path or sets PGDATA; 18 refuses the first and the second skips the move")
	}
	script, err := os.ReadFile(filepath.Join("..", "..", "scripts", "postgres-17-to-18.sh"))
	if err != nil {
		t.Fatal(err)
	}
	// The kit's sha256; every Ky repo vendors the same bytes.
	if sum := fmt.Sprintf("%x", sha256.Sum256(script)); sum != "195ae8684e34dd515158a88b1f76b918cf6e3f3c854ce83fe4fb190a5178cf06" {
		t.Errorf("scripts/postgres-17-to-18.sh differs from the shared kit (sha256 %s)", sum)
	}
}
