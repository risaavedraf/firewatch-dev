#!/usr/bin/env python3
"""Verify the FireWatch synthetic scenario workflow in a disposable Docker stack.

This harness is deliberately separate from ``scripts/smoke.py``. ``smoke.py``
exercises the operational Compose stack and reads the protected ``.env``; this
harness never reads ``.env``, never touches an existing stack, and generates all
of its credentials in memory.

What it does, always from the public HTTP surface:

1. builds isolated gateway/jobs/risk images from the repository build context,
   which ``.dockerignore`` already excludes ``.env`` from (enforced below);
2. starts one uniquely named container each of PostgreSQL/PostGIS, RabbitMQ and
   Keycloak on a private network, with init and realm files mounted read-only
   from ``deploy/local``;
3. runs the forward migrations as an ephemeral PostgreSQL administrator through
   each service image's default ``migrate`` entrypoint;
4. starts the three services bound to loopback ports only;
5. authenticates ``demo-admin`` and ``demo-viewer`` and checks the scenario
   workflow end to end (401/403, catalogue, rejections, five publishable
   profiles, legacy fixture, idempotent retry);
6. removes only the resources it created, in a ``finally`` block.

It proves an authenticated HTTP + RabbitMQ + PostGIS workflow against synthetic
inputs. It makes no calibrated-model, trained-metric or real-observation claim,
and it does not cover browser/PWA behaviour. Tokens and passwords are generated
in memory and are never printed or logged.

Run ``python backend/scripts/verify_synthetic.py --help`` for the exact options.
"""
from __future__ import annotations

import argparse
import datetime
import http.client
import json
import os
import re
import secrets
import shutil
import socket
import subprocess
import sys
import time
import urllib.parse
import uuid
from collections.abc import Callable
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parent.parent

POSTGIS_IMAGE = "postgis/postgis:17-3.5"
RABBIT_IMAGE = "rabbitmq:4.1.8-management"
KEYCLOAK_IMAGE = "quay.io/keycloak/keycloak:26.5.2"

SERVICES = {
    "gateway": ROOT / "services" / "gateway" / "Dockerfile",
    "jobs": ROOT / "services" / "jobs" / "Dockerfile",
    "risk": ROOT / "services" / "risk" / "Dockerfile",
}

REALM = "firewatch"
CLIENT_ID = "firewatch-demo"
AUDIENCE = "firewatch-api"
KEYCLOAK_DISCOVERY_PATH = f"/realms/{REALM}/.well-known/openid-configuration"

# The shared wire vocabulary and the frozen academic-rule-v1 fingerprint. The
# harness asserts these so a silent classifier or vocabulary drift fails loudly.
SCENARIO_IDS = ["mild", "mixed", "dry-windy", "missing-humidity", "stale", "partial-retrieval"]
PUBLISHABLE_SCENARIOS = ["mild", "mixed", "dry-windy", "missing-humidity", "stale"]
EXPECTED_LEVELS = {
    "mild": "bajo",
    "mixed": "medio",
    "dry-windy": "alto",
    "missing-humidity": "desconocido",
    "stale": "desconocido",
}
FACTOR_NAMES = {"temperatura", "humedad_relativa", "viento", "cobertura_vegetal", "incendios_historicos"}
FROZEN_POLICY_ID = "academic-rule-v1"
FROZEN_PARAMS_HASH = "e17aaa5600eb67c9a91340e90057aec100f4e80ef0895662f373cffee66564c7"
CANONICAL_LEVELS = {"bajo", "medio", "alto", "desconocido"}

DEFAULT_TIMEOUT = 900
DEFAULT_READY_TIMEOUT = 240
DEFAULT_POLL_INTERVAL = 1
DEFAULT_ZONE = "valparaiso-demo"
HTTP_TIMEOUT = 15
PARTIAL_RETRIEVAL_WAIT = 30


class HarnessError(RuntimeError):
    """A bounded, operator-actionable failure whose message never contains a secret."""


def _positive_seconds(raw: str) -> int:
    try:
        value = int(raw)
    except (TypeError, ValueError):
        raise argparse.ArgumentTypeError("must be a positive whole number of seconds") from None
    if value <= 0:
        raise argparse.ArgumentTypeError("must be a positive whole number of seconds")
    return value


def parse_args(argv=None):
    parser = argparse.ArgumentParser(
        prog="verify_synthetic.py",
        description=(
            "Verify the FireWatch synthetic scenario workflow in a disposable Docker stack. "
            "Builds isolated gateway/jobs/risk images, starts private PostgreSQL/PostGIS, RabbitMQ "
            "and Keycloak containers, runs authenticated HTTP checks, and removes only its own resources."
        ),
    )
    parser.add_argument(
        "--timeout",
        type=_positive_seconds,
        default=DEFAULT_TIMEOUT,
        metavar="SECONDS",
        help=f"overall deadline for the whole run (default: {DEFAULT_TIMEOUT})",
    )
    parser.add_argument(
        "--ready-timeout",
        type=_positive_seconds,
        default=DEFAULT_READY_TIMEOUT,
        metavar="SECONDS",
        help=f"per-service readiness deadline (default: {DEFAULT_READY_TIMEOUT})",
    )
    parser.add_argument(
        "--poll-interval",
        type=_positive_seconds,
        default=DEFAULT_POLL_INTERVAL,
        metavar="SECONDS",
        help=f"seconds between readiness probes (default: {DEFAULT_POLL_INTERVAL})",
    )
    parser.add_argument(
        "--zone",
        default=DEFAULT_ZONE,
        metavar="ZONE",
        help=f"catalogue zone used by the workflow (default: {DEFAULT_ZONE})",
    )
    parser.add_argument(
        "--check-partial-retrieval",
        action="store_true",
        help="wait a bounded time and confirm the partial-retrieval job is never published (off by default)",
    )
    parser.add_argument(
        "--keep",
        action="store_true",
        help="leave the created containers, network and images in place for inspection instead of cleaning up",
    )
    return parser.parse_args(argv)


def free_loopback_ports(count: int) -> list[int]:
    """Return ``count`` distinct currently free TCP ports on 127.0.0.1."""
    sockets = []
    try:
        for _ in range(count):
            sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            sock.bind(("127.0.0.1", 0))
            sockets.append(sock)
        return [int(sock.getsockname()[1]) for sock in sockets]
    finally:
        for sock in sockets:
            sock.close()


def _container_path(path: Path) -> str:
    """Return a portable absolute path for a Docker bind mount."""
    return str(path.resolve()).replace("\\", "/")


def _decode(raw: bytes) -> Any:
    if not raw:
        return None
    try:
        return json.loads(raw)
    except ValueError:
        return None


def _parse_instant(value: Any) -> datetime.datetime | None:
    if not isinstance(value, str) or not value.strip():
        return None
    text = value.strip()
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"
    try:
        parsed = datetime.datetime.fromisoformat(text)
    except ValueError:
        return None
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=datetime.UTC)
    return parsed.astimezone(datetime.UTC)


def _is_instant(value: Any) -> bool:
    return _parse_instant(value) is not None


def _same_instant(left: Any, right: Any) -> bool:
    a, b = _parse_instant(left), _parse_instant(right)
    return a is not None and a == b


def _http_exchange(url: str, *, method: str = "GET", headers: dict[str, str] | None = None, data: bytes | None = None, timeout: int = HTTP_TIMEOUT) -> tuple[int, bytes]:
    """Open exactly one HTTP(S) exchange and return ``(status, body)``.

    The scheme and host are validated explicitly, so a URL that is not plain
    HTTP(S) to a named host is refused before any socket is opened.
    """
    parsed = urllib.parse.urlsplit(url)
    if parsed.scheme not in ("http", "https") or not parsed.hostname:
        raise HarnessError("refusing to open a URL that is not HTTP(S) to a named host")
    path = parsed.path or "/"
    if parsed.query:
        path += "?" + parsed.query
    connect = http.client.HTTPSConnection if parsed.scheme == "https" else http.client.HTTPConnection
    connection = connect(parsed.hostname, parsed.port, timeout=timeout)
    try:
        connection.request(method, path, body=data, headers=headers or {})
        response = connection.getresponse()
        return response.status, response.read()
    finally:
        connection.close()


def http_call(
    base: str,
    path: str,
    *,
    method: str = "GET",
    token: str | None = None,
    body=None,
    idem_key: str | None = None,
    timeout: int = HTTP_TIMEOUT,
    retries: int = 2,
) -> tuple[int, Any]:
    """Perform one public HTTP call. Never logs the token or the body.

    Returns ``(status, parsed_json_or_none)``. An HTTP error status is returned,
    not raised; a transport failure raises HarnessError so readiness probes can
    retry it through their own bounded loop.
    """
    headers: dict[str, str] = {}
    if token:
        headers["Authorization"] = "Bearer " + token
    data = None
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    if idem_key:
        headers["Idempotency-Key"] = idem_key
    attempt = 0
    while True:
        try:
            status, raw = _http_exchange(base.rstrip("/") + path, method=method, headers=headers, data=data, timeout=timeout)
        except (OSError, http.client.HTTPException) as error:
            raise HarnessError(f"{method} {path} could not reach the stack ({type(error).__name__})") from None
        if status == 429 and attempt < retries:
            attempt += 1
            time.sleep(1)
            continue
        return status, _decode(raw)


def keycloak_issuer(port: int) -> str:
    """Return the issuer Keycloak advertises for the firewatch realm."""
    return f"http://localhost:{port}/realms/{REALM}"


def keycloak_discovery_url(port: int) -> str:
    """Return the realm discovery URL on the assigned Keycloak application port."""
    return f"http://127.0.0.1:{port}{KEYCLOAK_DISCOVERY_PATH}"


def keycloak_ready(port: int, timeout: int = 5) -> bool:
    """Report whether the realm discovery document is served with the expected issuer.

    Keycloak 26 serves the management health endpoint on a separate port that
    this harness does not publish, so readiness is proven on the application port
    the services actually use: an HTTP 200 discovery document whose ``issuer``
    equals ``http://localhost:<port>/realms/firewatch``.
    """
    try:
        status, raw = _http_exchange(keycloak_discovery_url(port), timeout=timeout)
    except (OSError, http.client.HTTPException):
        return False
    if status != 200:
        return False
    payload = _decode(raw)
    return isinstance(payload, dict) and payload.get("issuer") == keycloak_issuer(port)


class Harness:
    def __init__(self, args):
        self.args = args
        self.deadline = time.monotonic() + args.timeout
        self.run_id = secrets.token_hex(6)
        self.prefix = f"firewatch-synthetic-verify-{self.run_id}"
        self.network = self.prefix + "-net"
        self.owned_container_ids: list[str] = []
        self.images: list[str] = []
        self.secrets = {
            "POSTGRES_USER": "postgres",
            "POSTGRES_PASSWORD": secrets.token_hex(16),
            "JOBS_DB_PASSWORD": secrets.token_hex(16),
            "RISK_DB_PASSWORD": secrets.token_hex(16),
            "KEYCLOAK_DB_PASSWORD": secrets.token_hex(16),
            "RABBITMQ_PASSWORD": secrets.token_hex(16),
            "KEYCLOAK_ADMIN_PASSWORD": secrets.token_hex(16),
            "DEMO_ADMIN_PASSWORD": secrets.token_hex(16),
            "DEMO_VIEWER_PASSWORD": secrets.token_hex(16),
        }
        # The only values that may ever appear in diagnostics are non-secret
        # literals; every generated credential is registered for redaction.
        self.redactions = list(self.secrets.values())
        self.gateway_port = 0
        self.identity_port = 0
        self.management_port = 0
        self.docker: str | None = None
        # Ownership is recorded only after a successful create; a generated name
        # alone never authorises a delete.
        self.owned_network: str | None = None

    # -- process helpers ---------------------------------------------------
    def _redact(self, text: str) -> str:
        for secret in sorted(self.redactions, key=len, reverse=True):
            if secret:
                text = text.replace(secret, "<redacted>")
        return text

    def _remaining(self) -> float:
        return self.deadline - time.monotonic()

    def _docker(self, argv, *, env=None, timeout=None):
        remaining = self._remaining()
        if timeout is not None:
            remaining = min(remaining, timeout)
        if remaining <= 0:
            raise HarnessError("overall timeout reached before a docker command could run")
        child = dict(os.environ)
        if env:
            child.update(env)
        if self.docker is None:
            raise HarnessError("docker binary was not resolved before a command was run")
        try:
            proc = subprocess.run(
                [self.docker, *argv],
                env=child,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                timeout=remaining,
            )
        except subprocess.TimeoutExpired:
            raise HarnessError("docker command timed out: " + " ".join(argv[:2])) from None
        output = self._redact(proc.stdout.decode("utf-8", "replace")) if proc.stdout else ""
        return proc.returncode, output

    def _docker_cleanup(self, argv, timeout=60) -> tuple[int, str]:
        """Run one cleanup command without the run deadline and report its result."""
        if self.docker is None:
            return 125, "docker binary was not resolved"
        try:
            proc = subprocess.run(
                [self.docker, *argv],
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                timeout=timeout,
            )
        except subprocess.TimeoutExpired:
            return 124, "cleanup command timed out"
        except OSError as error:
            return 125, type(error).__name__
        output = self._redact(proc.stdout.decode("utf-8", "replace")) if proc.stdout else ""
        return proc.returncode, output

    def _wait_for(self, predicate: Callable[[], bool], description: str):
        timeout = min(self.args.ready_timeout, max(1.0, self._remaining()))
        deadline = time.monotonic() + timeout
        last = None
        while time.monotonic() < deadline:
            try:
                if predicate():
                    return
            except HarnessError as error:
                last = self._redact(str(error))
            except Exception as error:  # noqa: BLE001 - readiness probes must never abort the run
                last = type(error).__name__
            time.sleep(self.args.poll_interval)
        detail = f" ({last})" if last else ""
        raise HarnessError(f"timed out after {timeout:.0f}s waiting for {description}{detail}")

    # -- environment -------------------------------------------------------
    def check_environment(self):
        # Resolve the Docker CLI once and reuse the full path everywhere, so every
        # call reaches the same binary.
        self.docker = shutil.which("docker")
        if self.docker is None:
            raise HarnessError("Docker CLI not found on PATH; install Docker and start its daemon first")
        if not (ROOT / "docker-compose.yml").is_file():
            raise HarnessError(f"unexpected project layout: {ROOT} is not the backend root")
        ignore = ROOT / ".dockerignore"
        if not ignore.is_file():
            raise HarnessError(".dockerignore is missing; refusing to build an image that could include .env")
        patterns = {
            line.strip()
            for line in ignore.read_text(encoding="utf-8").splitlines()
            if line.strip() and not line.strip().startswith("#")
        }
        if ".env" not in patterns and "**/.env" not in patterns:
            raise HarnessError(".dockerignore does not exclude .env; refusing to build an image that could contain secrets")
        for name, dockerfile in SERVICES.items():
            if not dockerfile.is_file():
                raise HarnessError(f"missing Dockerfile for {name}: {dockerfile}")
        code, output = self._docker(["info"], timeout=30)
        if code != 0:
            raise HarnessError("Docker daemon is not reachable: " + output.strip()[-500:])
        for image in (POSTGIS_IMAGE, RABBIT_IMAGE, KEYCLOAK_IMAGE):
            code, _ = self._docker(["image", "inspect", image])
            if code != 0:
                raise HarnessError(f"required image {image} is not present locally; pull it before running this harness")

    # -- resource lifecycle ------------------------------------------------
    def build_images(self):
        for name, dockerfile in SERVICES.items():
            tag = f"{self.prefix}-{name}:latest"
            code, output = self._docker(["build", "--tag", tag, "--file", str(dockerfile), str(ROOT)])
            if code != 0:
                raise HarnessError(f"docker build failed for {name}:\n{output.strip()[-2000:]}")
            self.images.append(tag)
        print("PASS: built isolated gateway, jobs and risk images from the repository context")

    def create_network(self):
        code, output = self._docker(
            [
                "network",
                "create",
                "--label",
                "firewatch.verify=1",
                "--label",
                f"firewatch.verify.run={self.run_id}",
                self.network,
            ]
        )
        if code != 0:
            raise HarnessError(f"failed to create the isolated network: {output.strip()[-500:]}")
        # Randomness is not ownership: only a successful create makes this run
        # responsible for the network, so cleanup can never delete a pre-existing
        # network that happened to share the generated name.
        self.owned_network = self.network

    @staticmethod
    def _extract_container_id(output: str, name: str) -> str:
        """Return the container id docker printed, or fail without a name fallback."""
        for line in reversed(output.splitlines()):
            candidate = line.strip()
            if re.fullmatch(r"[0-9a-f]{12,64}", candidate):
                return candidate
        raise HarnessError(f"container {name} was created but no container id was returned")

    def _start_container(self, name, image, *, alias=None, env=None, ports=(), mounts=(), command=None, hostname=None):
        argv = [
            "run",
            "--detach",
            "--name",
            name,
            "--label",
            "firewatch.verify=1",
            "--label",
            f"firewatch.verify.run={self.run_id}",
            "--network",
            self.network,
        ]
        if alias:
            argv += ["--network-alias", alias]
        if hostname:
            argv += ["--hostname", hostname]
        for port in ports:
            argv += ["--publish", port]
        for source, target in mounts:
            argv += ["--mount", f"type=bind,source={source},target={target},readonly"]
        for variable in (env or {}):
            argv += ["-e", variable]
        argv.append(image)
        if command:
            argv += list(command)
        code, output = self._docker(argv, env=env)
        if code != 0:
            raise HarnessError(f"failed to start container {name}: {output.strip()[-1000:]}")
        container_id = self._extract_container_id(output, name)
        self.owned_container_ids.append(container_id)
        return container_id

    def _run_owned_container(self, name, image, *, env=None, command=None):
        """Create, record ownership, then run one short-lived container to completion.

        The container is named and labelled before it starts, and it is recorded
        as owned immediately after a successful create, so a start failure or an
        overall timeout still lets cleanup remove exactly this container.
        """
        argv = [
            "create",
            "--name",
            name,
            "--label",
            "firewatch.verify=1",
            "--label",
            f"firewatch.verify.run={self.run_id}",
            "--network",
            self.network,
        ]
        for variable in (env or {}):
            argv += ["-e", variable]
        argv.append(image)
        if command:
            argv += list(command)
        code, output = self._docker(argv, env=env)
        if code != 0:
            raise HarnessError(f"failed to create container {name}: {output.strip()[-1000:]}")
        container_id = self._extract_container_id(output, name)
        self.owned_container_ids.append(container_id)
        code, output = self._docker(["start", "--attach", container_id])
        if code != 0:
            raise HarnessError(f"container {name} ({container_id}) exited with failure:\n{output.strip()[-1500:]}")

    def start_infrastructure(self):
        # Three distinct loopback ports. Keycloak's advertised hostname is the
        # same host:port as the issuer the services validate against.
        self.gateway_port, self.identity_port, self.management_port = free_loopback_ports(3)

        self._start_container(
            self.prefix + "-postgres",
            POSTGIS_IMAGE,
            alias="postgres",
            env={
                "POSTGRES_USER": self.secrets["POSTGRES_USER"],
                "POSTGRES_PASSWORD": self.secrets["POSTGRES_PASSWORD"],
                "JOBS_DB_PASSWORD": self.secrets["JOBS_DB_PASSWORD"],
                "RISK_DB_PASSWORD": self.secrets["RISK_DB_PASSWORD"],
                "KEYCLOAK_DB_PASSWORD": self.secrets["KEYCLOAK_DB_PASSWORD"],
            },
            mounts=[
                (_container_path(ROOT / "deploy" / "local" / "postgres-init.sh"), "/docker-entrypoint-initdb.d/10-firewatch.sh"),
            ],
        )
        self._start_container(
            self.prefix + "-rabbitmq",
            RABBIT_IMAGE,
            alias="rabbitmq",
            hostname="rabbitmq",
            env={
                "RABBITMQ_DEFAULT_USER": "firewatch",
                "RABBITMQ_DEFAULT_PASS": self.secrets["RABBITMQ_PASSWORD"],
                "RABBITMQ_DEFAULT_VHOST": "firewatch",
            },
            ports=[f"127.0.0.1:{self.management_port}:15672"],
            mounts=[
                (_container_path(ROOT / "deploy" / "local" / "rabbitmq-plugins"), "/etc/rabbitmq/enabled_plugins"),
            ],
        )

    def start_identity(self):
        # Keycloak needs the identity database from the init script, so it is
        # started only after PostgreSQL has reported the databases ready.
        self._start_container(
            self.prefix + "-identity",
            KEYCLOAK_IMAGE,
            alias="identity",
            env={
                "KC_DB": "postgres",
                "KC_DB_URL": "jdbc:postgresql://postgres:5432/identity",
                "KC_DB_USERNAME": "identity_app",
                "KC_DB_PASSWORD": self.secrets["KEYCLOAK_DB_PASSWORD"],
                "KC_BOOTSTRAP_ADMIN_USERNAME": "local-admin",
                "KC_BOOTSTRAP_ADMIN_PASSWORD": self.secrets["KEYCLOAK_ADMIN_PASSWORD"],
                "KC_HOSTNAME": f"http://localhost:{self.identity_port}",
                "DEMO_ADMIN_PASSWORD": self.secrets["DEMO_ADMIN_PASSWORD"],
                "DEMO_VIEWER_PASSWORD": self.secrets["DEMO_VIEWER_PASSWORD"],
            },
            ports=[f"127.0.0.1:{self.identity_port}:8080"],
            mounts=[
                (_container_path(ROOT / "deploy" / "local" / "firewatch-realm.json"), "/opt/keycloak/data/import/firewatch-realm.json"),
            ],
            command=["start-dev", "--import-realm"],
        )

    def wait_for_postgres(self):
        def ready():
            code, output = self._docker(
                [
                    "exec",
                    self.prefix + "-postgres",
                    "psql",
                    "-h",
                    "127.0.0.1",
                    "-U",
                    "postgres",
                    "-d",
                    "postgres",
                    "-tAc",
                    "SELECT count(*) FROM pg_database WHERE datname IN ('jobs','risk','identity')",
                ]
            )
            return code == 0 and output.strip() == "3"

        self._wait_for(ready, "PostgreSQL with the jobs/risk/identity databases")

    def wait_for_rabbitmq(self):
        def ready():
            code, _ = self._docker(["exec", self.prefix + "-rabbitmq", "rabbitmq-diagnostics", "-q", "ping"])
            return code == 0

        self._wait_for(ready, "RabbitMQ")

    def wait_for_keycloak(self):
        # Keycloak 26 serves health on a separate management port, so readiness
        # is the realm discovery document on the application port: it proves the
        # realm is imported and its issuer matches the value the services check.
        self._wait_for(
            lambda: keycloak_ready(self.identity_port, timeout=5),
            f"Keycloak realm discovery at {keycloak_discovery_url(self.identity_port)}",
        )

    def run_migrations(self):
        for database in ("jobs", "risk"):
            url = (
                f"postgres://postgres:{self.secrets['POSTGRES_PASSWORD']}@postgres:5432/{database}?sslmode=disable"
            )
            self._run_owned_container(
                f"{self.prefix}-{database}-migrate",
                f"{self.prefix}-{database}:latest",
                env={"DATABASE_URL": url},
                command=["migrate"],
            )
        print("PASS: forward migrations applied as the ephemeral PostgreSQL administrator")

    def start_services(self):
        issuer = f"http://localhost:{self.identity_port}/realms/{REALM}"
        common = {
            "APP_ENV": "development",
            "OIDC_ISSUER": issuer,
            "OIDC_JWKS_URL": f"http://identity:8080/realms/{REALM}/protocol/openid-connect/certs",
            "OIDC_AUDIENCE": AUDIENCE,
        }
        amqp = f"amqp://firewatch:{self.secrets['RABBITMQ_PASSWORD']}@rabbitmq:5672/firewatch"
        self._start_container(
            self.prefix + "-jobs",
            f"{self.prefix}-jobs:latest",
            alias="jobs",
            env={
                **common,
                "DATABASE_URL": f"postgres://jobs_app:{self.secrets['JOBS_DB_PASSWORD']}@postgres:5432/jobs?sslmode=disable",
                "AMQP_URL": amqp,
            },
        )
        self._start_container(
            self.prefix + "-risk",
            f"{self.prefix}-risk:latest",
            alias="risk",
            env={
                **common,
                "DATABASE_URL": f"postgres://risk_app:{self.secrets['RISK_DB_PASSWORD']}@postgres:5432/risk?sslmode=disable",
                "AMQP_URL": amqp,
            },
        )
        self._start_container(
            self.prefix + "-gateway",
            f"{self.prefix}-gateway:latest",
            alias="gateway",
            env={
                **common,
                "JOBS_URL": "http://jobs:8080",
                "RISK_URL": "http://risk:8080",
                # Required by the gateway at startup, like JOBS_URL and RISK_URL.
                # This harness starts no prevention container and calls no
                # /v1/prevention/ route, so the value only has to exist.
                "PREVENTION_URL": "http://prevention:8084",
                "WEB_ORIGIN": "http://localhost:4173",
            },
            ports=[f"127.0.0.1:{self.gateway_port}:8080"],
        )

    def wait_for_gateway(self):
        url = f"http://127.0.0.1:{self.gateway_port}/health/ready"

        def ready():
            try:
                status, _ = _http_exchange(url, timeout=5)
                return status == 204
            except (OSError, http.client.HTTPException):
                return False

        self._wait_for(ready, "gateway health endpoint")

    def fetch_token(self, user: str, password: str) -> str:
        url = f"http://127.0.0.1:{self.identity_port}/realms/{REALM}/protocol/openid-connect/token"
        body = urllib.parse.urlencode(
            {"grant_type": "password", "client_id": CLIENT_ID, "username": user, "password": password}
        ).encode("utf-8")
        try:
            status, raw = _http_exchange(
                url,
                method="POST",
                headers={"Content-Type": "application/x-www-form-urlencoded"},
                data=body,
                timeout=HTTP_TIMEOUT,
            )
        except (OSError, http.client.HTTPException):
            raise HarnessError(f"the token endpoint for {user} is unreachable") from None
        if status != 200:
            raise HarnessError(f"token request for {user} was rejected with HTTP {status}")
        payload = _decode(raw)
        if not isinstance(payload, dict):
            raise HarnessError(f"the token response for {user} was not a JSON object")
        token = payload.get("access_token")
        if not token:
            raise HarnessError(f"the token response for {user} carried no access token")
        self.redactions.append(token)
        return token

    def wait_for_api(self, admin: str):
        base = self.gateway_base()
        self._wait_for(
            lambda: http_call(base, "/v1/risk/synthetic-scenarios", token=admin)[0] == 200,
            "risk service through the gateway",
        )
        self._wait_for(
            lambda: http_call(base, "/v1/audit", token=admin)[0] == 200,
            "jobs service through the gateway",
        )

    # -- verification ------------------------------------------------------
    def gateway_base(self) -> str:
        return f"http://127.0.0.1:{self.gateway_port}"

    @staticmethod
    def _expect(condition, message):
        if not condition:
            raise HarnessError(message)

    @staticmethod
    def _expect_status(status, expected, label):
        if status != expected:
            raise HarnessError(f"{label}: expected HTTP {expected}, got {status}")

    def poll_published(self, base: str, token: str, job_id: str) -> Any:
        timeout = min(self.args.ready_timeout, max(1.0, self._remaining()))
        deadline = time.monotonic() + timeout
        last = None
        while time.monotonic() < deadline:
            status, job = http_call(base, f"/v1/jobs/{job_id}", token=token)
            if status == 200 and isinstance(job, dict):
                if job.get("state") == "published":
                    return job
                last = job.get("state")
            else:
                last = f"HTTP {status}"
            time.sleep(self.args.poll_interval)
        raise HarnessError(f"job {job_id} was not published within the deadline (last state: {last})")

    def find_batch(self, base: str, token: str, zone: str, job_id: str) -> Any:
        status, history = http_call(base, f"/v1/risk/history?zone_id={urllib.parse.quote(zone)}", token=token)
        if status != 200 or not isinstance(history, list):
            raise HarnessError(f"history read failed while looking for job {job_id}")
        for batch in history:
            if batch.get("job_id") == job_id:
                return batch
        raise HarnessError(f"no published batch was found for job {job_id}")

    def verify(self, admin: str, viewer: str):
        base = self.gateway_base()
        zone = self.args.zone
        zone_query = urllib.parse.quote(zone)

        # 1. Unauthenticated access is refused at the gateway.
        self._expect_status(http_call(base, "/v1/jobs")[0], 401, "unauthenticated GET /v1/jobs")
        self._expect_status(http_call(base, f"/v1/risk/latest?zone_id={zone_query}")[0], 401, "unauthenticated GET /v1/risk/latest")
        print("PASS: gateway rejects unauthenticated requests with 401")

        # 2. The authenticated catalogue exposes exactly the six profiles.
        status, viewer_catalog = http_call(base, "/v1/risk/synthetic-scenarios", token=viewer)
        self._expect_status(status, 200, "viewer scenario catalogue")
        self._expect(
            isinstance(viewer_catalog, list) and [entry.get("id") for entry in viewer_catalog] == SCENARIO_IDS,
            "viewer catalogue is not the six known profiles in the contract order",
        )
        self._expect(
            all(isinstance(entry.get("description"), str) and entry["description"].strip() for entry in viewer_catalog),
            "a scenario catalogue entry has no description",
        )
        status, admin_catalog = http_call(base, "/v1/risk/synthetic-scenarios", token=admin)
        self._expect_status(status, 200, "admin scenario catalogue")
        self._expect(
            [entry.get("id") for entry in admin_catalog] == SCENARIO_IDS,
            "admin catalogue differs from the viewer catalogue",
        )
        print("PASS: authenticated scenario catalogue returns six profiles for viewer and admin")

        # 3. PostGIS is exercised through the authenticated zone catalogue.
        status, zones = http_call(base, "/v1/risk/zones", token=viewer)
        self._expect_status(status, 200, "zone catalogue")
        self._expect(isinstance(zones, dict) and zones.get("type") == "FeatureCollection", "zone catalogue is not a GeoJSON FeatureCollection")
        features = zones.get("features") or []
        self._expect(len(features) >= 30, f"zone catalogue exposes too few geometry rows: {len(features)}")
        self._expect(
            all(isinstance(feature.get("geometry"), dict) and feature["geometry"].get("type") for feature in features),
            "a zone row carries no PostGIS geometry",
        )
        print("PASS: authenticated PostGIS zone catalogue returns GeoJSON geometries")

        # 4. A viewer cannot create jobs.
        self._expect_status(
            http_call(base, "/v1/jobs", method="POST", token=viewer, body={"zone_id": zone}, idem_key=str(uuid.uuid4()))[0],
            403,
            "viewer POST /v1/jobs",
        )
        print("PASS: viewer is forbidden from creating jobs with 403")

        # 5. Explicitly invalid scenario metadata is rejected, and the public
        #    audit/latest/history surfaces stay unchanged under decoded-JSON equality.
        status, audit_before = http_call(base, "/v1/audit", token=admin)
        self._expect_status(status, 200, "audit read")
        _, latest_before = http_call(base, f"/v1/risk/latest?zone_id={zone_query}", token=viewer)
        status, history_before = http_call(base, f"/v1/risk/history?zone_id={zone_query}", token=viewer)
        self._expect_status(status, 200, "history read")

        invalid_requests = [
            ("client evaluated_at", {"zone_id": zone, "evaluated_at": "2026-01-01T00:00:00Z"}),
            ("unknown scenario", {"zone_id": zone, "scenario_id": "not-a-profile"}),
            ("null scenario", {"zone_id": zone, "scenario_id": None}),
            ("empty scenario", {"zone_id": zone, "scenario_id": ""}),
            ("non-string scenario", {"zone_id": zone, "scenario_id": 123}),
        ]
        for label, payload in invalid_requests:
            status, body = http_call(base, "/v1/jobs", method="POST", token=admin, body=payload, idem_key=str(uuid.uuid4()))
            self._expect_status(status, 400, f"{label} rejection")
            self._expect(
                isinstance(body, dict) and body.get("code") == "invalid_demo_request",
                f"{label} did not return the invalid_demo_request code",
            )
        print("PASS: evaluated_at/unknown/null/empty scenario requests return invalid_demo_request (400)")

        _, audit_after = http_call(base, "/v1/audit", token=admin)
        self._expect(audit_after == audit_before, "a rejected request changed the public audit log")
        _, latest_after = http_call(base, f"/v1/risk/latest?zone_id={zone_query}", token=viewer)
        self._expect(latest_after == latest_before, "a rejected request changed the latest batch")
        _, history_after = http_call(base, f"/v1/risk/history?zone_id={zone_query}", token=viewer)
        self._expect(history_after == history_before, "a rejected request changed the batch history")
        print("PASS: rejected requests created no job, no audit entry and no batch")

        # 6. Each publishable profile publishes its documented level and carries
        #    the same scenario/instant identity on job, batch and history.
        for scenario in PUBLISHABLE_SCENARIOS:
            accepted = self._expect_accepted(base, admin, {"zone_id": zone, "scenario_id": scenario}, scenario)
            completed = self.poll_published(base, admin, accepted["id"])
            self._expect(completed.get("scenario_id") == scenario, f"{scenario}: completed job lost its scenario")
            self._expect(
                _same_instant(completed.get("evaluated_at"), accepted.get("evaluated_at")),
                f"{scenario}: completed job changed its evaluation instant",
            )
            batch = self.find_batch(base, viewer, zone, accepted["id"])
            self._check_batch(batch, scenario, accepted.get("evaluated_at"))
            status, latest = http_call(base, f"/v1/risk/latest?zone_id={zone_query}", token=viewer)
            self._expect_status(status, 200, f"{scenario} latest read")
            self._expect(latest.get("id") == completed.get("batch_id"), f"{scenario}: latest batch is not the published one")
            self._expect(latest.get("scenario_id") == scenario, f"{scenario}: latest batch lost its scenario")
        print("PASS: five publishable profiles publish bajo/medio/alto/desconocido with five factors, rationale and the frozen policy fingerprint")

        # 7. The partial retrieval is accepted by the transport but must never be
        #    published. Waiting is optional because it is a queued-not-terminal
        #    state today.
        accepted = self._expect_accepted(base, admin, {"zone_id": zone, "scenario_id": "partial-retrieval"}, "partial-retrieval")
        if self.args.check_partial_retrieval:
            self._assert_never_published(base, admin, accepted["id"])
            print("PASS: partial-retrieval stayed unpublished while the bounded wait elapsed")
        else:
            print("SKIP: partial-retrieval publication not runtime-checked (enable --check-partial-retrieval)")

        # 8. A request without a scenario keeps the legacy fixture and publishes
        #    no synthetic metadata.
        legacy = self._expect_accepted(base, admin, {"zone_id": zone}, "legacy")
        self._expect(
            "scenario_id" not in legacy and "evaluated_at" not in legacy,
            "a legacy job exposed synthetic metadata",
        )
        completed = self.poll_published(base, admin, legacy["id"])
        self._expect(
            "scenario_id" not in completed and "evaluated_at" not in completed,
            "a legacy completed job exposed synthetic metadata",
        )
        batch = self.find_batch(base, viewer, zone, legacy["id"])
        self._expect(
            "scenario_id" not in batch and "evaluated_at" not in batch,
            "a legacy batch exposed synthetic metadata",
        )
        self._expect(batch.get("risk_level") in CANONICAL_LEVELS, "a legacy batch carried a non-canonical level")
        print("PASS: a request without scenario_id keeps the legacy fixture and publishes without synthetic metadata")

        # 9. An idempotent retry with a different valid scenario returns the
        #    original job identity and adds no batch or audit row.
        key = str(uuid.uuid4())
        status, first = http_call(base, "/v1/jobs", method="POST", token=admin, body={"zone_id": zone, "scenario_id": "mild"}, idem_key=key)
        self._expect_status(status, 202, "idempotent job accepted")
        status, retry = http_call(base, "/v1/jobs", method="POST", token=admin, body={"zone_id": zone, "scenario_id": "dry-windy"}, idem_key=key)
        self._expect_status(status, 202, "idempotent retry accepted")
        self._expect(retry.get("id") == first.get("id"), "the idempotent retry created a new job")
        self._expect(retry.get("scenario_id") == "mild", "the idempotent retry replaced the original scenario")
        self._expect(_same_instant(retry.get("evaluated_at"), first.get("evaluated_at")), "the idempotent retry replaced the original instant")
        completed = self.poll_published(base, admin, first["id"])
        status, history = http_call(base, f"/v1/risk/history?zone_id={zone_query}", token=viewer)
        self._expect_status(status, 200, "history read after the idempotent retry")
        matches = [batch for batch in history if batch.get("job_id") == first["id"]]
        self._expect(len(matches) == 1, f"the idempotent retry produced {len(matches)} batches for one job")
        self._expect(matches[0].get("scenario_id") == "mild", "the single batch does not carry the original scenario")
        _, audit = http_call(base, "/v1/audit", token=admin)
        actions = sorted(entry.get("action") for entry in audit if entry.get("job_id") == first["id"])
        self._expect(
            actions == ["batch.published", "job.requested"],
            f"unexpected audit trail for the idempotent job: {actions!r}",
        )
        self._expect(completed.get("batch_id") == matches[0].get("id"), "the completed job does not name the single batch")
        print("PASS: an idempotent retry with a different scenario returns the original job and adds no batch or audit row")

        print("PASS: all synthetic verification checks succeeded")
        print("NOTE: synthetic demonstration only; no calibrated model, trained metric or real observation was verified.")

    def _expect_accepted(self, base: str, admin: str, body: Any, scenario: str) -> Any:
        status, accepted = http_call(base, "/v1/jobs", method="POST", token=admin, body=body, idem_key=str(uuid.uuid4()))
        self._expect_status(status, 202, f"{scenario} job accepted")
        self._expect(isinstance(accepted, dict) and accepted.get("id"), f"{scenario}: accepted job has no id")
        if scenario != "legacy":
            self._expect(accepted.get("scenario_id") == scenario, f"{scenario}: accepted job lost its scenario")
            self._expect(_is_instant(accepted.get("evaluated_at")), f"{scenario}: accepted job has no evaluated_at")
        return accepted

    def _check_batch(self, batch: Any, scenario: str, instant: Any):
        self._expect(batch.get("scenario_id") == scenario, f"{scenario}: batch scenario mismatch")
        self._expect(_same_instant(batch.get("evaluated_at"), instant), f"{scenario}: batch instant mismatch")
        self._expect(
            batch.get("risk_level") == EXPECTED_LEVELS[scenario],
            f"{scenario}: risk level {batch.get('risk_level')!r} != {EXPECTED_LEVELS[scenario]!r}",
        )
        self._expect(batch.get("demo") is True, f"{scenario}: batch demo flag is not true")
        factors = batch.get("factors")
        self._expect(isinstance(factors, list) and len(factors) == 5, f"{scenario}: batch does not carry five factors")
        self._expect({factor.get("name") for factor in factors} == FACTOR_NAMES, f"{scenario}: factor names differ from the canonical set")
        self._expect(
            isinstance(batch.get("rationale"), str) and batch["rationale"].strip(),
            f"{scenario}: batch has no rationale",
        )
        policy = batch.get("policy")
        self._expect(
            isinstance(policy, dict) and policy.get("policy_id") == FROZEN_POLICY_ID,
            f"{scenario}: batch policy id is not {FROZEN_POLICY_ID}",
        )
        self._expect(policy.get("params_hash") == FROZEN_PARAMS_HASH, f"{scenario}: batch policy fingerprint is not the frozen one")
        self._expect(batch.get("model_version") == FROZEN_POLICY_ID, f"{scenario}: batch model version is not the documented rule")
        self._expect(
            isinstance(batch.get("geometry"), dict) and batch["geometry"].get("type") == "Point",
            f"{scenario}: batch has no PostGIS point geometry",
        )

    def _assert_never_published(self, base: str, admin: str, job_id: str):
        timeout = min(PARTIAL_RETRIEVAL_WAIT, self.args.ready_timeout, max(1.0, self._remaining()))
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            status, job = http_call(base, f"/v1/jobs/{job_id}", token=admin)
            if status == 200 and isinstance(job, dict) and job.get("state") == "published":
                raise HarnessError("partial-retrieval was published; an incomplete retrieval must never produce a batch")
            time.sleep(self.args.poll_interval)

    # -- orchestration -----------------------------------------------------
    def run(self):
        self.check_environment()
        failure: HarnessError | None = None
        try:
            self.build_images()
            self.create_network()
            self.start_infrastructure()
            self.wait_for_postgres()
            self.wait_for_rabbitmq()
            self.start_identity()
            self.run_migrations()
            self.start_services()
            self.wait_for_keycloak()
            self.wait_for_gateway()
            admin = self.fetch_token("demo-admin", self.secrets["DEMO_ADMIN_PASSWORD"])
            viewer = self.fetch_token("demo-viewer", self.secrets["DEMO_VIEWER_PASSWORD"])
            self.wait_for_api(admin)
            self.verify(admin, viewer)
        except HarnessError as error:
            failure = error
        finally:
            cleanup_failures = self.cleanup()
        if failure is not None:
            message = str(failure)
            if cleanup_failures:
                message = f"{message}; cleanup also failed: {'; '.join(cleanup_failures)}"
            raise HarnessError(message)
        if cleanup_failures:
            raise HarnessError("cleanup failed: " + "; ".join(cleanup_failures))

    def cleanup(self) -> list[str]:
        if self.args.keep:
            print(
                f"KEEP: leaving resources in place: network={self.network} containers={','.join(self.owned_container_ids)}",
                file=sys.stderr,
            )
            return []
        failures: list[str] = []
        for container_id in reversed(self.owned_container_ids):
            code, output = self._docker_cleanup(["rm", "--force", container_id])
            if code != 0:
                failures.append(f"container {container_id}: {output.strip()[-200:] or f'exit {code}'}")
        if self.owned_network:
            code, output = self._docker_cleanup(["network", "rm", self.owned_network])
            if code != 0:
                failures.append(f"network {self.owned_network}: {output.strip()[-200:] or f'exit {code}'}")
        for tag in self.images:
            code, output = self._docker_cleanup(["rmi", "--force", tag])
            if code != 0:
                failures.append(f"image {tag}: {output.strip()[-200:] or f'exit {code}'}")
        return failures


def main(argv=None) -> int:
    args = parse_args(argv)
    harness = Harness(args)
    try:
        harness.run()
    except HarnessError as error:
        print("FAIL: " + harness._redact(str(error)), file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        print("FAIL: interrupted", file=sys.stderr)
        return 130
    return 0


if __name__ == "__main__":
    sys.exit(main())
