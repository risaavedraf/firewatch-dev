#!/usr/bin/env python3
"""Verify the CI migration readiness contract on a disposable Compose project.

``Backend integration`` starts ``postgres`` and then the ``*-test-migrate``
services, which declare ``postgres: {condition: service_healthy}``. The Compose
healthcheck is therefore the only gate between the container booting and a
migrator dialling it, and ``jobs-test``/``risk-test`` depend on both the
migrators and the test databases they create.

This harness checks that gate directly, on its own project:

1. pins the manifest, project directory, project name and an *empty* temporary
   env file on every command, so the caller's directory, a hostile
   ``COMPOSE_FILE``/``COMPOSE_PROJECT_NAME`` override and the developer's
   ``backend/.env`` cannot change what runs;
2. confirms the generated project name is unknown to the daemon, starts one
   ``postgres`` service from it with every credential generated in memory, and
   records the resources the daemon reports under that project's Compose label;
3. waits for Docker to report the container ``healthy``;
4. at that instant, asserts the readiness rule the migrators rely on: a TCP
   connection inside the container (``pg_isready -h 127.0.0.1``) must succeed;
5. runs ``jobs-test-migrate`` and ``risk-test-migrate`` and requires exit 0;
6. asserts the test databases accept a connection as the application roles the
   database suites use (``jobs_app``/``jobs_test``, ``risk_app``/``risk_test``);
7. removes what it created, on its own budget, in a ``finally`` block: a pinned
   ``compose down``, then per-resource deletion only for resources whose Compose
   label it can still confirm as its own, then a check that nothing is left.

The failure it is built to catch: the PostgreSQL entrypoint runs
``docker-entrypoint-initdb.d`` against a temporary server started with
``listen_addresses=''``, which answers on the local Unix socket. A socket-only
``pg_isready`` therefore reports the container healthy before the real server
listens on TCP and before the databases exist, and the migrators exit 1 with
``migration connection failed``. That is the failure observed in run
``39161170125``; see ``docs/ci-migration-readiness.md``.

Contract:

* stdout carries progress, the final ``PASS``/``FAIL``/``ERROR`` line and any
  ``cleanup error:`` lines; every line is redacted, so no generated credential
  can appear there. ``argparse`` usage errors go to stderr as usual.
* the exit code is the result: ``0`` the contract holds, ``1`` the contract was
  violated, ``2`` the harness could not run the check, ``3`` the contract held
  but cleanup failed. A cleanup failure never replaces a failure above it.

It proves the readiness contract only. It makes no claim about the rest of the
integration job, it never reads ``backend/.env``, and it never touches the
operational ``firewatch`` project.

Run ``python backend/scripts/verify_ci_migration.py --help`` for the options.
"""
from __future__ import annotations

import argparse
import json
import os
import secrets
import shutil
import subprocess
import sys
import tempfile
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
COMPOSE_FILE = ROOT / "docker-compose.yml"

# The CLI description is the docstring's first line, kept typed for argparse.
DESCRIPTION = "Verify the CI migration readiness contract on a disposable Compose project."

# Namespaced so a stray project can never be confused with the operational stack.
PROJECT_PREFIX = "firewatch-ci-migration-verify"

# Compose labels every resource it creates with this key; it is the identity that
# authorises a delete, never the project name on its own.
PROJECT_LABEL = "com.docker.compose.project"

# Compose honours these from the environment, so they are stripped from every
# child instead of being trusted. Everything else (DOCKER_HOST, PATH, ...) passes
# through, because reaching the daemon is not the harness's decision.
COMPOSE_ENV_PREFIX = "COMPOSE_"

# Every credential docker-compose.yml requires by `${NAME:?required}`.
PASSWORD_KEYS = (
    "POSTGRES_PASSWORD",
    "JOBS_DB_PASSWORD",
    "RISK_DB_PASSWORD",
    "KEYCLOAK_DB_PASSWORD",
    "RABBITMQ_PASSWORD",
    "KEYCLOAK_ADMIN_PASSWORD",
    "DEMO_ADMIN_PASSWORD",
    "DEMO_VIEWER_PASSWORD",
    "GRAFANA_ADMIN_PASSWORD",
)

# (migrator service, test database, application role, password key): what the
# test profile must be able to reach the moment postgres is reported healthy.
TEST_PROFILE_TARGETS = (
    ("jobs-test-migrate", "jobs_test", "jobs_app", "JOBS_DB_PASSWORD"),
    ("risk-test-migrate", "risk_test", "risk_app", "RISK_DB_PASSWORD"),
)

# How a resource of each kind is inspected for its Compose label and deleted.
# The label value is read as JSON, which keeps the argument free of quoting.
LABEL_FORMAT = {
    "container": "{{json .Config.Labels}}",
    "network": "{{json .Labels}}",
    "volume": "{{json .Labels}}",
}
INSPECT_ARGV = {
    "container": ["inspect"],
    "network": ["network", "inspect"],
    "volume": ["volume", "inspect"],
}
DELETE_ARGV = {
    "container": ["rm", "-f"],
    "network": ["network", "rm"],
    "volume": ["volume", "rm"],
}

HEALTH_POLL_SECONDS = 0.2

EXIT_OK = 0
EXIT_CONTRACT_VIOLATION = 1
EXIT_HARNESS_ERROR = 2
EXIT_CLEANUP_FAILED = 3


class HarnessError(RuntimeError):
    """The harness could not run the check at all (missing docker, layout, budget)."""


class ContractViolation(AssertionError):
    """The readiness contract was violated; the migrators would have failed."""


class MigrationReadinessProbe:
    """Owns one disposable Compose project and the checks run against it."""

    def __init__(self, timeout: int, cleanup_timeout: int, keep: bool):
        self.keep = keep
        self.cleanup_timeout = cleanup_timeout
        self.verification_deadline = time.monotonic() + timeout
        self.cleanup_deadline: float | None = None
        self.project = f"{PROJECT_PREFIX}-{secrets.token_hex(6)}"
        self.secrets = {key: secrets.token_hex(24) for key in PASSWORD_KEYS}
        # Only non-secret literals may reach diagnostics, so every generated
        # credential is registered for redaction.
        self.redactions = list(self.secrets.values())
        self.docker = shutil.which("docker")
        # Ownership is the set of resources the daemon reports under this
        # project's label; a generated name alone never authorises a delete.
        self.owned: dict[str, list[str]] = {}
        self.ownership_errors: list[str] = []
        # Only a started lifecycle may be cleaned up.
        self.attempted = False
        handle, self.env_file = tempfile.mkstemp(prefix=f"{PROJECT_PREFIX}-", suffix=".env")
        os.close(handle)

    # -- process helpers ---------------------------------------------------
    def _redact(self, text: str) -> str:
        for secret in sorted(self.redactions, key=len, reverse=True):
            text = text.replace(secret, "<redacted>")
        return text

    def _remaining(self, budget: str) -> float:
        """Seconds left in the named budget, opening it on first use."""
        if budget == "cleanup":
            if self.cleanup_deadline is None:
                self.cleanup_deadline = time.monotonic() + self.cleanup_timeout
            return self.cleanup_deadline - time.monotonic()
        return self.verification_deadline - time.monotonic()

    def _child_env(self) -> dict[str, str]:
        env = {
            key: value
            for key, value in os.environ.items()
            if not key.startswith(COMPOSE_ENV_PREFIX)
        }
        env.update(self.secrets)
        return env

    def _run(self, argv: list[str], timeout: float, budget: str = "verify") -> tuple[int, str]:
        """Run one command in the repository root with a scrubbed environment."""
        remaining = self._remaining(budget)
        if remaining <= 0:
            raise HarnessError(f"the {budget} budget was exhausted before a command could run")
        try:
            proc = subprocess.run(
                argv,
                cwd=str(ROOT),
                env=self._child_env(),
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                timeout=max(1.0, min(timeout, remaining)),
            )
        except subprocess.TimeoutExpired:
            # One failure is final: a timed-out command is never retried.
            raise HarnessError(f"command timed out within the {budget} budget: " + " ".join(argv[1:3])) from None
        output = self._redact(proc.stdout.decode("utf-8", "replace")) if proc.stdout else ""
        return proc.returncode, output

    def _docker(self, argv: list[str], timeout: float = 60, budget: str = "verify") -> tuple[int, str]:
        """Run a raw ``docker`` command, pinned to the same root and environment."""
        if self.docker is None:
            raise HarnessError("docker was not resolved before a command was run")
        return self._run([self.docker, *argv], timeout, budget)

    def compose(self, argv: list[str], timeout: float = 300, budget: str = "verify") -> tuple[int, str]:
        """Run `docker compose` with the manifest, directory and project pinned.

        The pinned arguments come first so that no inherited ``COMPOSE_*`` value
        and no caller directory can decide which file, project or env file runs.
        """
        if self.docker is None:
            raise HarnessError("docker was not resolved before a command was run")
        return self._run(
            [
                self.docker,
                "compose",
                "--project-directory",
                str(ROOT),
                "-f",
                str(COMPOSE_FILE),
                "--env-file",
                self.env_file,
                "--project-name",
                self.project,
                *argv,
            ],
            timeout,
            budget,
        )

    def check_environment(self) -> None:
        if self.docker is None:
            raise HarnessError("docker is not on PATH; this check needs a Docker daemon")
        if not COMPOSE_FILE.is_file():
            raise HarnessError(f"unexpected project layout: {COMPOSE_FILE} is missing")

    def _require(self, code: int, output: str, what: str) -> None:
        if code != 0:
            raise HarnessError(f"{what} failed with exit {code}:\n{output}")

    # -- ownership ---------------------------------------------------------
    def project_resources(self, budget: str = "verify") -> dict[str, list[str]]:
        """List the resources the daemon reports under this project's label."""
        filters = ["--filter", f"label={PROJECT_LABEL}={self.project}", "-q"]
        listings = {
            "container": ["ps", "-a", *filters],
            "network": ["network", "ls", *filters],
            "volume": ["volume", "ls", *filters],
        }
        found: dict[str, list[str]] = {}
        for kind, argv in listings.items():
            code, output = self._docker(argv, timeout=60, budget=budget)
            if code != 0:
                raise HarnessError(
                    f"cannot list {kind}s of project {self.project} (exit {code}):\n{output}"
                )
            found[kind] = [line.strip() for line in output.splitlines() if line.strip()]
        return found

    def preflight(self) -> None:
        """Refuse a project name the daemon already knows anything about."""
        existing = self.project_resources()
        if any(existing.values()):
            raise HarnessError(
                f"project {self.project} already exists ({existing}); "
                "refusing to reuse or delete resources this run did not create"
            )
        print(f"preflight: project {self.project} is empty")

    def refresh_owned(self) -> None:
        """Record what exists for this project, including a partial creation."""
        try:
            self.owned = self.project_resources()
        except HarnessError as failure:
            self.owned = {}
            self.ownership_errors.append(str(failure))

    def _resource_is_owned(self, kind: str, resource: str) -> bool:
        """True only if the daemon still reports our label on this resource."""
        argv = [*INSPECT_ARGV[kind], "-f", LABEL_FORMAT[kind], resource]
        code, output = self._docker(argv, timeout=60, budget="cleanup")
        if code != 0 or not output.strip():
            return False
        try:
            labels = json.loads(output.strip())
        except ValueError:
            return False
        return isinstance(labels, dict) and labels.get(PROJECT_LABEL) == self.project

    def _delete(self, kind: str, resource: str) -> tuple[int, str]:
        return self._docker([*DELETE_ARGV[kind], resource], timeout=120, budget="cleanup")

    # -- lifecycle ---------------------------------------------------------
    def start_postgres(self) -> None:
        self.attempted = True
        code, output = self.compose(["--profile", "test", "up", "-d", "--no-deps", "postgres"])
        # A failed start can still leave containers, volumes or networks behind,
        # so ownership is recorded before the exit code is judged.
        self.refresh_owned()
        self._require(code, output, "starting the disposable postgres service")

    def postgres_container(self) -> str:
        code, output = self.compose(["ps", "-a", "-q", "postgres"])
        self._require(code, output, "locating the disposable postgres container")
        container = output.strip()
        if not container:
            raise HarnessError("the disposable postgres container was not found")
        return container

    def health_status(self, container: str) -> str:
        code, output = self._docker(
            ["inspect", "-f", "{{.State.Health.Status}}", container], timeout=30
        )
        if code != 0:
            return "unknown"
        return output.strip()

    def wait_until_healthy(self, container: str) -> float:
        started = time.monotonic()
        while time.monotonic() - started < self._remaining("verify"):
            status = self.health_status(container)
            if status == "healthy":
                return time.monotonic() - started
            if status in {"unhealthy", "unknown"}:
                _, log = self.compose(["logs", "--no-color", "--tail", "40", "postgres"], timeout=60)
                raise HarnessError(f"postgres reported {status} instead of becoming healthy:\n{log}")
            time.sleep(HEALTH_POLL_SECONDS)
        raise HarnessError("postgres did not report healthy before the overall timeout")

    # -- checks ------------------------------------------------------------
    def assert_tcp_ready(self) -> None:
        """`healthy` must mean the real server accepts TCP connections."""
        code, output = self.compose(
            [
                "exec", "-T", "postgres",
                "pg_isready", "-h", "127.0.0.1", "-p", "5432", "-U", "postgres", "-d", "postgres",
            ],
            timeout=60,
        )
        if code != 0:
            raise ContractViolation(
                "compose reported postgres healthy, but a TCP check inside the container failed "
                f"(exit {code}):\n{output}\nThe healthcheck is answering before the real server "
                "listens on TCP, so a migrator that starts now cannot connect."
            )

    def assert_migrator_succeeds(self, service: str) -> None:
        """Run one test migrator at the instant postgres is reported healthy."""
        code, output = self.compose(["--profile", "test", "run", "--build", "--rm", service], timeout=900)
        if code == 0:
            return
        _, logs = self.compose(["logs", "--no-color", "--tail", "40", service], timeout=60)
        raise ContractViolation(
            f"{service} exited {code} although postgres was reported healthy; "
            f"the exact CI failure this check guards against:\n{output}\n--- {service} log ---\n{logs}"
        )

    def assert_role_can_connect(self, database: str, role: str, password_key: str) -> None:
        code, output = self.compose(
            [
                "exec", "-T", "-e", f"PGPASSWORD={self.secrets[password_key]}", "postgres",
                "psql", "-h", "127.0.0.1", "-U", role, "-d", database, "-tAc", "SELECT 1",
            ],
            timeout=60,
        )
        if code != 0 or "1" not in output:
            raise ContractViolation(
                f"{role} cannot connect to {database} over TCP although postgres is healthy "
                f"(exit {code}):\n{output}\nThe test suite for that database would fail."
            )

    def verify(self) -> None:
        """The whole readiness contract, raising on the first violation."""
        self.check_environment()
        print(f"project: {self.project}")
        self.preflight()
        self.start_postgres()
        container = self.postgres_container()
        elapsed = self.wait_until_healthy(container)
        print(f"postgres reported healthy {elapsed:.1f}s after start")
        # No wait here on purpose: the contract is about this exact instant.
        self.assert_tcp_ready()
        print("healthy implies TCP accepts connections")
        for service, _, _, _ in TEST_PROFILE_TARGETS:
            self.assert_migrator_succeeds(service)
            print(f"{service}: exit 0")
        for _, database, role, password_key in TEST_PROFILE_TARGETS:
            self.assert_role_can_connect(database, role, password_key)
            print(f"{role} reached {database} over TCP")
        print("PASS: migration readiness contract holds")

    # -- cleanup -----------------------------------------------------------
    def cleanup(self) -> list[str]:
        """Remove only owned resources; return every error instead of raising."""
        errors = list(self.ownership_errors)
        if self.keep:
            print(f"cleanup skipped for {self.project} (kept: --keep)")
            return errors
        if not self.attempted:
            print(f"cleanup: nothing was created for {self.project}")
            return errors
        code, output = self.compose(
            ["--profile", "test", "down", "-v", "--remove-orphans"],
            timeout=self.cleanup_timeout,
            budget="cleanup",
        )
        print(f"cleanup of {self.project}: compose down exit {code}")
        if code != 0:
            errors.append(f"compose down exited {code}: {output}")
        try:
            remaining = self.project_resources(budget="cleanup")
        except HarnessError as failure:
            errors.append(str(failure))
            remaining = {}
        for kind, resources in remaining.items():
            for resource in resources:
                if not self._resource_is_owned(kind, resource):
                    errors.append(
                        f"refusing to delete {kind} {resource}: its {PROJECT_LABEL} label could "
                        f"not be confirmed as {self.project}"
                    )
                    continue
                code, output = self._delete(kind, resource)
                if code != 0:
                    errors.append(f"removing {kind} {resource} exited {code}: {output}")
        try:
            leftover = self.project_resources(budget="cleanup")
        except HarnessError as failure:
            errors.append(str(failure))
            leftover = {}
        if any(leftover.values()):
            errors.append(f"resources are still present after cleanup: {leftover}")
        return errors

    def remove_env_file(self) -> None:
        # missing_ok keeps this safe when the harness never created the file.
        Path(self.env_file).unlink(missing_ok=True)


def positive_seconds(raw: str) -> int:
    try:
        value = int(raw)
    except ValueError:
        raise argparse.ArgumentTypeError("must be a whole number of seconds") from None
    if value <= 0:
        raise argparse.ArgumentTypeError("must be a positive whole number of seconds")
    return value


def parse_args(argv=None):
    parser = argparse.ArgumentParser(description=DESCRIPTION)
    parser.add_argument(
        "--timeout",
        type=positive_seconds,
        default=900,
        help="budget in seconds for the readiness checks (default: 900)",
    )
    parser.add_argument(
        "--cleanup-timeout",
        type=positive_seconds,
        default=300,
        help="separate budget in seconds for the cleanup that always follows (default: 300)",
    )
    parser.add_argument(
        "--keep",
        action="store_true",
        help="keep the disposable project for inspection instead of removing it",
    )
    return parser.parse_args(argv)


def main(argv=None) -> int:
    args = parse_args(argv)
    try:
        probe = MigrationReadinessProbe(
            timeout=args.timeout, cleanup_timeout=args.cleanup_timeout, keep=args.keep
        )
    except OSError as failure:
        print(f"ERROR: cannot create the temporary env file: {failure}")
        return EXIT_HARNESS_ERROR
    status = EXIT_HARNESS_ERROR
    cleanup_errors: list[str] = []
    try:
        try:
            probe.verify()
            status = EXIT_OK
        except ContractViolation as failure:
            print(f"FAIL: {failure}")
            status = EXIT_CONTRACT_VIOLATION
        except HarnessError as failure:
            print(f"ERROR: {failure}")
            status = EXIT_HARNESS_ERROR
    finally:
        # Cleanup runs on its own budget, never hides the result above, and the
        # temporary env file is dropped even when cleanup itself fails.
        try:
            cleanup_errors = probe.cleanup()
        except Exception as failure:  # noqa: BLE001 - cleanup must not hide the result
            cleanup_errors = [f"cleanup failed: {type(failure).__name__}: {failure}"]
        finally:
            probe.remove_env_file()
    for message in cleanup_errors:
        print(f"cleanup error: {message}")
    if cleanup_errors:
        return status if status != EXIT_OK else EXIT_CLEANUP_FAILED
    return status


if __name__ == "__main__":
    sys.exit(main())
