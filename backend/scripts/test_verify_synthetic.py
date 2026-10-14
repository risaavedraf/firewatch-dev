"""Public-CLI and adapter tests for ``verify_synthetic.py``.

The tests run the harness as a subprocess. Argument/help tests use a PATH with
no Docker binary; lifecycle tests use an isolated *fake* Docker CLI that records
every invocation and satisfies the readiness probes in memory.

Safety invariant: before a lifecycle test launches the harness, it proves that
``shutil.which("docker", path=<fake PATH>)`` resolves exactly the owned fake
executable. The fake PATH contains *only* the fake bin directory, so no host
Docker can be reached, and the fake itself invokes the absolute
``sys.executable``. If the gate cannot prove this, the test fails closed and the
harness is never launched. No test ever starts real Docker, builds an image or
runs a container.

Run with::

    python -m unittest discover -s backend/scripts -p test_verify_synthetic.py -v
"""
from __future__ import annotations

import hashlib
import http.server
import json
import os
import shutil
import subprocess
import sys
import tempfile
import threading
import unittest
from pathlib import Path
from typing import cast

SCRIPT = Path(__file__).resolve().parent / "verify_synthetic.py"

# The public module seams the readiness probe is allowed to expose.
sys.path.insert(0, str(SCRIPT.parent))
import verify_synthetic  # noqa: E402  (public readiness adapter under test)

# The exact validation message the harness must emit for a bad --timeout.
TIMEOUT_ERROR = "argument --timeout: must be a positive whole number of seconds"


class FakeIsolationError(RuntimeError):
    """The fake Docker CLI could not be proven as the only resolvable docker."""


def assert_fake_docker_only(path_value: str, expected: Path) -> str:
    """Return the resolved docker path, or fail closed if it is not the owned fake.

    This only performs a path lookup; it never executes the resolved binary.
    """

    resolved = shutil.which("docker", path=path_value)
    if resolved is None:
        raise FakeIsolationError("no docker executable resolves on the supplied PATH")
    if os.path.normcase(os.path.abspath(resolved)) != os.path.normcase(os.path.abspath(str(expected))):
        raise FakeIsolationError(f"resolved docker {resolved!r} is not the owned fake {expected}")
    return resolved


def fake_container_id(name: str) -> str:
    """Mirror of the fake Docker CLI's deterministic 64-hex container id."""
    return hashlib.sha256(name.encode("utf-8")).hexdigest()


# A fake Docker CLI. It records every invocation as one JSON line, returns a
# deterministic container id derived from ``--name`` for create/run, and decides
# success/failure/sleep from a comma-separated FAKE_DOCKER_SCENARIO value.
# This is a raw string so the ``\n`` escapes reach the written script verbatim.
FAKE_DOCKER = r'''\
import hashlib
import json
import os
import sys
import time

_log = os.environ["FAKE_DOCKER_LOG"]
_scenario = set(filter(None, os.environ.get("FAKE_DOCKER_SCENARIO", "").split(",")))
_argv = sys.argv[1:]
with open(_log, "a", encoding="utf-8") as _handle:
    _handle.write(json.dumps(_argv) + "\n")


def _container_id():
    for index, part in enumerate(_argv):
        if part == "--name" and index + 1 < len(_argv):
            return hashlib.sha256(_argv[index + 1].encode("utf-8")).hexdigest()
    seed = "anon-" + " ".join(_argv)
    return hashlib.sha256(seed.encode("utf-8")).hexdigest()


if not _argv:
    sys.exit(0)
_command = _argv[0]
if _command == "network" and len(_argv) > 1:
    if _argv[1] == "create":
        if "network_create" in _scenario:
            sys.stderr.write("network create failed\n")
            sys.exit(1)
        print("fake-network-id")
        sys.exit(0)
    if _argv[1] == "rm":
        sys.exit(1 if "network_rm" in _scenario else 0)
if _command == "start":
    if "start_sleep" in _scenario:
        # Simulate an attached start that outlives the harness subprocess bound.
        # Closing both pipe fds keeps the timeout prompt when the process is killed.
        for _fd in (1, 2):
            try:
                os.close(_fd)
            except OSError:
                pass
        time.sleep(6)
        os._exit(0)
    if "start" in _scenario:
        sys.stderr.write("start failed\n")
        sys.exit(1)
    sys.exit(0)
if _command == "rm":
    sys.exit(1 if "rm" in _scenario else 0)
if _command == "rmi":
    sys.exit(1 if "rmi" in _scenario else 0)
if _command == "exec" and any(part == "psql" for part in _argv):
    sys.stdout.write("3\n")
    sys.exit(0)
if _command == "exec":
    sys.exit(0)
if _command == "create":
    if "create" in _scenario:
        sys.stderr.write("container name already in use\n")
        sys.exit(1)
    print(_container_id())
    sys.exit(0)
if _command == "run":
    print(_container_id())
    sys.exit(0)
sys.exit(0)
'''


def path_without_docker() -> str:
    """Return PATH with every directory that contains a Docker binary removed."""
    entries = []
    for entry in os.environ.get("PATH", "").split(os.pathsep):
        if not entry:
            continue
        if (Path(entry) / "docker").exists() or (Path(entry) / "docker.exe").exists():
            continue
        entries.append(entry)
    return os.pathsep.join(entries)


def run_script(args, *, cwd=None):
    env = dict(os.environ)
    env["PATH"] = path_without_docker()
    return subprocess.run(
        [sys.executable, str(SCRIPT), *args],
        capture_output=True,
        text=True,
        env=env,
        cwd=cwd,
    )


class FakeDocker:
    """An isolated fake ``docker`` executable that records every call."""

    def __init__(self, root: Path, scenario: str):
        self.bin = root / "fakebin"
        self.bin.mkdir()
        self.log = root / "docker-calls.jsonl"
        self.log.write_text("", encoding="utf-8")
        self.scenario = scenario
        (self.bin / "fake_docker.py").write_text(FAKE_DOCKER, encoding="utf-8")
        (self.bin / "docker.cmd").write_text(
            f'@echo off\r\n"{sys.executable}" "%~dp0fake_docker.py" %*\r\n', encoding="utf-8"
        )
        posix = self.bin / "docker"
        posix.write_text(f'#!/bin/sh\nexec "{sys.executable}" "$(dirname "$0")/fake_docker.py" "$@"\n', encoding="utf-8")
        posix.chmod(0o755)
        self.exe = self.bin / ("docker.cmd" if os.name == "nt" else "docker")

    def assert_isolated(self) -> str:
        return assert_fake_docker_only(str(self.bin), self.exe)

    def env(self) -> dict:
        # Fail closed before any harness launch if the fake is not the only docker.
        self.assert_isolated()
        env = dict(os.environ)
        # Only the owned fake bin is on PATH: no host Docker can be reached.
        env["PATH"] = str(self.bin)
        env["FAKE_DOCKER_LOG"] = str(self.log)
        env["FAKE_DOCKER_SCENARIO"] = self.scenario
        return env

    def calls(self) -> list[list[str]]:
        return [json.loads(line) for line in self.log.read_text(encoding="utf-8").splitlines() if line.strip()]


class HelpAndArgumentTests(unittest.TestCase):
    def test_help_exits_zero_and_advertises_real_options(self):
        result = run_script(["--help"])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("usage:", result.stdout)
        for option in (
            "--timeout",
            "--ready-timeout",
            "--poll-interval",
            "--zone",
            "--check-partial-retrieval",
            "--keep",
        ):
            self.assertIn(option, result.stdout, f"{option} missing from --help")

    def test_invalid_timeout_exits_two_with_exact_error(self):
        for raw in ("0", "-5", "abc"):
            with self.subTest(raw=raw):
                result = run_script(["--timeout", raw])
                self.assertEqual(result.returncode, 2, result.stderr)
                self.assertIn(TIMEOUT_ERROR, result.stderr)
                self.assertNotIn("Traceback", result.stderr)

    def test_rejected_arguments_leave_an_isolated_directory_untouched(self):
        with tempfile.TemporaryDirectory() as tmp:
            result = run_script(["--timeout", "abc"], cwd=tmp)
            self.assertEqual(result.returncode, 2)
            self.assertNotIn("Traceback", result.stderr)
            self.assertEqual(os.listdir(tmp), [])

    def test_unknown_option_is_rejected_without_side_effects(self):
        with tempfile.TemporaryDirectory() as tmp:
            result = run_script(["--not-a-real-option"], cwd=tmp)
            self.assertEqual(result.returncode, 2)
            self.assertNotIn("Traceback", result.stderr)
            self.assertEqual(os.listdir(tmp), [])

    def test_missing_docker_is_reported_without_side_effects(self):
        with tempfile.TemporaryDirectory() as tmp:
            result = run_script([], cwd=tmp)
            self.assertNotEqual(result.returncode, 0)
            self.assertNotIn("Traceback", result.stderr)
            self.assertIn("Docker", result.stderr)
            self.assertEqual(os.listdir(tmp), [])


class FakeIsolationTests(unittest.TestCase):
    """The fake-only gate must accept the owned fake and reject everything else."""

    def test_gate_accepts_the_owned_fake(self):
        with tempfile.TemporaryDirectory() as tmp:
            fake = FakeDocker(Path(tmp), "ok")
            resolved = assert_fake_docker_only(str(fake.bin), fake.exe)
            self.assertEqual(
                os.path.normcase(os.path.abspath(resolved)),
                os.path.normcase(os.path.abspath(str(fake.exe))),
            )

    def test_gate_rejects_a_missing_executable(self):
        with tempfile.TemporaryDirectory() as tmp, self.assertRaises(FakeIsolationError):
            assert_fake_docker_only(tmp, Path(tmp) / "docker")

    def test_gate_rejects_a_foreign_executable(self):
        with tempfile.TemporaryDirectory() as tmp, self.assertRaises(FakeIsolationError):
            foreign = Path(tmp) / ("docker.cmd" if os.name == "nt" else "docker")
            foreign.write_text("", encoding="utf-8")
            foreign.chmod(0o755)
            assert_fake_docker_only(tmp, Path(tmp) / "another-docker")

    def test_gate_rejects_the_host_docker_as_fallback(self):
        owned = Path(tempfile.gettempdir()) / "definitely-not-the-owned-fake-docker"
        with self.assertRaises(FakeIsolationError):
            assert_fake_docker_only(os.environ.get("PATH", ""), owned)


class _DiscoveryServer(http.server.HTTPServer):
    """An HTTP server that records requests and can answer the realm discovery document."""

    def __init__(self, address):
        super().__init__(address, _DiscoveryHandler)
        self.requested: list[str] = []
        self.expected_path: str = verify_synthetic.KEYCLOAK_DISCOVERY_PATH
        self.issuer: str | None = None


class _DiscoveryHandler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        server = cast(_DiscoveryServer, self.server)
        server.requested.append(self.path)
        if self.path == server.expected_path and server.issuer:
            body = json.dumps({"issuer": server.issuer}).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, format, *args):
        return


class KeycloakReadinessTests(unittest.TestCase):
    """The readiness probe must target the Keycloak application port discovery."""

    def setUp(self):
        self.server = _DiscoveryServer(("127.0.0.1", 0))
        self.port = self.server.server_address[1]
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()

    def test_discovery_url_targets_the_assigned_application_port(self):
        url = verify_synthetic.keycloak_discovery_url(self.port)
        self.assertEqual(url, f"http://127.0.0.1:{self.port}{verify_synthetic.KEYCLOAK_DISCOVERY_PATH}")
        self.assertIn("/.well-known/openid-configuration", url)
        self.assertNotIn(":9000", url)

    def test_ready_requires_the_realm_issuer_on_the_application_port(self):
        self.assertFalse(verify_synthetic.keycloak_ready(self.port, timeout=2))
        self.server.issuer = verify_synthetic.keycloak_issuer(self.port)
        self.assertTrue(verify_synthetic.keycloak_ready(self.port, timeout=2))
        self.assertIn(verify_synthetic.KEYCLOAK_DISCOVERY_PATH, self.server.requested)

    def test_ready_rejects_a_mismatched_issuer(self):
        self.server.issuer = "http://localhost:1/realms/other"
        self.assertFalse(verify_synthetic.keycloak_ready(self.port, timeout=2))


class ResourceOwnershipTests(unittest.TestCase):
    """Only resources this run successfully created may be removed."""

    def _run(self, scenario, timeout="8"):
        with tempfile.TemporaryDirectory() as tmp:
            fake = FakeDocker(Path(tmp), scenario)
            env = fake.env()  # fails closed unless the fake is the only docker
            args = ["--timeout", timeout, "--ready-timeout", "1", "--poll-interval", "1"]
            result = subprocess.run(
                [sys.executable, str(SCRIPT), *args],
                capture_output=True,
                text=True,
                env=env,
                cwd=tmp,
            )
            return result, fake.calls()

    @staticmethod
    def _names(calls, command):
        return [call[call.index("--name") + 1] for call in calls if call and call[0] == command and "--name" in call]

    @staticmethod
    def _targets(calls, command, flag):
        return [call[2] for call in calls if len(call) >= 3 and call[0] == command and call[1] == flag]

    def test_failed_network_create_is_never_removed(self):
        result, calls = self._run("network_create")
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertTrue(any(call[:2] == ["network", "create"] for call in calls), calls)
        self.assertFalse(any(call[:2] == ["network", "rm"] for call in calls), calls)

    def test_created_migration_uses_and_cleans_up_the_returned_id(self):
        result, calls = self._run("start")
        self.assertEqual(result.returncode, 1, result.stderr)
        migrations = [name for name in self._names(calls, "create") if name.endswith("-migrate")]
        self.assertTrue(migrations, f"no named migration container was created: {calls}")
        started = self._targets(calls, "start", "--attach")
        removed = self._targets(calls, "rm", "--force")
        for name in migrations:
            container_id = fake_container_id(name)
            self.assertIn(container_id, started, f"start did not use the created id for {name}: {calls}")
            self.assertIn(container_id, removed, f"cleanup did not use the created id for {name}: {calls}")

    def test_failed_migration_create_never_deletes_a_preexisting_name(self):
        result, calls = self._run("create")
        self.assertEqual(result.returncode, 1, result.stderr)
        attempted = self._names(calls, "create")
        self.assertTrue(attempted, f"no migration create was attempted: {calls}")
        removed = self._targets(calls, "rm", "--force")
        started = self._targets(calls, "start", "--attach")
        for name in attempted:
            container_id = fake_container_id(name)
            self.assertNotIn(container_id, removed, f"{name} was removed although this run never owned it: {calls}")
            self.assertNotIn(container_id, started, f"{name} was started although its create failed: {calls}")

    def test_attached_start_timeout_cleans_up_the_created_id(self):
        result, calls = self._run("start_sleep", timeout="5")
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertIn("timed out", result.stderr.lower())
        self.assertNotIn("PASS: all synthetic verification checks succeeded", result.stdout)
        migrations = [name for name in self._names(calls, "create") if name.endswith("-migrate")]
        self.assertTrue(migrations, f"no named migration container was created: {calls}")
        started = self._targets(calls, "start", "--attach")
        removed = self._targets(calls, "rm", "--force")
        for name in migrations:
            container_id = fake_container_id(name)
            self.assertIn(container_id, started, f"start did not use the created id for {name}: {calls}")
            self.assertIn(container_id, removed, f"timed-out container was not cleaned up by id: {calls}")

    def test_cleanup_failure_is_reported_not_hidden(self):
        result, _ = self._run("rmi")
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertNotIn("PASS: all synthetic verification checks succeeded", result.stdout)
        self.assertIn("cleanup", result.stderr.lower())


if __name__ == "__main__":
    unittest.main()
