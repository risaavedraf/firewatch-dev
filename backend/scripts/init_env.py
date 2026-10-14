"""Create local demo credentials without overwriting existing configuration."""
from pathlib import Path
import os
import secrets

root = Path(__file__).resolve().parent.parent
names = [
    "POSTGRES_PASSWORD", "JOBS_DB_PASSWORD", "RISK_DB_PASSWORD",
    "KEYCLOAK_DB_PASSWORD", "RABBITMQ_PASSWORD", "KEYCLOAK_ADMIN_PASSWORD",
    "DEMO_ADMIN_PASSWORD", "DEMO_VIEWER_PASSWORD",
]
path = root / ".env"
fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
with os.fdopen(fd, "w", encoding="utf-8", newline="\n") as stream:
    stream.write("# Local credentials. Never commit this file.\n")
    for name in names:
        stream.write(f"{name}={secrets.token_hex(24)}\n")
print("Created backend/.env with random credentials; existing files are never overwritten.")
