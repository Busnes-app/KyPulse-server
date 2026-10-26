"""Check resolved profile overlays with synthetic inputs; start no containers."""

import json
import os
from pathlib import Path
import subprocess
import tempfile

root = Path(__file__).resolve().parents[1]
with tempfile.TemporaryDirectory(prefix="kypulse-profile-check-") as directory:
    fixture = Path(directory) / ".env"
    fixture.write_text(
        "KYPULSE_APP_URL=https://pulse.example.invalid\n"
        "KYPULSE_SESSION_SECRET=synthetic-pulse-profile-secret\n"
        "KYPULSE_DNS=192.0.2.53\n"
        "KYPULSE_CONTAINER_IP=192.0.2.10\n"
        "KYPULSE_NETWORK_SUBNET=192.0.2.0/24\n"
    )
    env = {key: value for key, value in os.environ.items()
           if not key.startswith(("COMPOSE_", "KYPULSE_", "POSTGRES_PASSWORD"))}

    def render(files):
        command = ["docker", "compose", "--project-name", "kypulse-profile-check",
                   "--env-file", str(fixture)]
        for file in files:
            command += ["-f", file]
        return json.loads(subprocess.check_output(command + ["config", "--format", "json"],
                                                 cwd=root, env=env))

    for overlays in [[], ["docker-compose.build.yml"], ["docker-compose.lan-dns.yml"],
                     ["docker-compose.build.yml", "docker-compose.lan-dns.yml",
                      "docker-compose.static-ip.yml"]]:
        files = ["docker-compose.yml", *overlays]
        baseline = render(files)
        configured = render([*files, "docker-compose.strong-crypto.yml"])
        assert configured["services"]["app"]["environment"].pop("GODEBUG") == "fips140=on"
        assert configured == baseline, "The profile changed another Compose setting"
print("All four chains retain data, credentials, image/build, network and DNS settings.")
