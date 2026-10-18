"""Regression tests for the backend Compose migration readiness contract.

``Backend integration`` cold-starts ``postgres`` and then runs the
``*-test-migrate`` services. Those declare ``postgres: {condition:
service_healthy}``, so the Compose healthcheck is the only thing between the
container booting and a migrator dialling it. The PostgreSQL entrypoint runs
``docker-entrypoint-initdb.d`` against a *temporary* server started with
``listen_addresses=''``; that temporary server answers on the Unix socket, which
is exactly the endpoint ``pg_isready`` probes when no host is given. A
socket-only healthcheck therefore reports the container healthy while the real
server is not listening on TCP and the databases do not exist yet: the migrators
connect, fail with ``migration connection failed`` and exit 1, and the job dies
with ``service "jobs-test-migrate" didn't complete successfully: exit 1``.

These tests pin the contract that closes that window without needing Docker:

* the postgres healthcheck must dial TCP instead of the Unix socket;
* every database the services connect to must be created by the mounted init
  helper, which the entrypoint runs before the real server starts listening;
* the test-profile services must keep waiting for their completed migrators; and
* the workflow must keep running the readiness checks and the test-profile gate
  without ``continue-on-error``.

The same file also drives the runtime harness end to end through a fake ``docker``
executable that is the only ``docker`` on the child's ``PATH`` and records every
invocation. That covers the harness's own contracts without a Docker daemon:

* every command pins the repository manifest, the project directory and the
  child cwd, and inherited ``COMPOSE_*`` overrides never reach it;
* a partially created project is still removed, and only resources that carry
  this project's Compose label are ever deleted;
* cleanup keeps its own budget, reports failures through the exit code without
  hiding the primary result, and always drops its temporary env file.

They read only shipped files. They never inspect Git history and never read
``.env``; the fake ``docker`` is a local script, not a container.

Run with::

    python -m unittest discover -s backend/scripts -p test_ci_migration.py -v
"""
from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
COMPOSE = ROOT / "docker-compose.yml"
POSTGRES_INIT = ROOT / "deploy" / "local" / "postgres-init.sh"
INTEGRATION_WORKFLOW = ROOT.parent / ".github" / "workflows" / "integration.yml"
VERIFY_SCRIPT = Path(__file__).resolve().parent / "verify_ci_migration.py"

# The services that run the database suites, and the migrator each one waits for.
TEST_PROFILE_GATES = {
    "jobs-test": "jobs-test-migrate",
    "risk-test": "risk-test-migrate",
    "prevention-test": "prevention-test-migrate",
}

TEST_PROFILE_MIGRATORS = (
    "jobs-test-migrate",
    "risk-test-migrate",
    "prevention-test-migrate",
)

# The exact gate commands the workflow must keep running, in the shipped form.
GATE_COMMANDS = (
    "docker compose --profile test run --build --rm jobs-test",
    "docker compose --profile test run --build --rm risk-test",
    "docker compose --profile test run --build --rm prevention-test",
)

READINESS_CHECKS = (
    "python3 -m unittest discover -s scripts -p test_ci_migration.py -v",
    "python3 scripts/verify_ci_migration.py",
)


def read(path: Path) -> str:
    """Return a shipped file's text, failing loudly if it moved."""
    if not path.is_file():
        raise AssertionError(f"expected {path} to exist")
    return path.read_text(encoding="utf-8")


def service_block(compose_text: str, name: str) -> str:
    """Return the YAML lines of one top-level service mapping.

    The compose file indents a service name by two spaces and its children by
    four or more, so the block ends at the next line that is neither blank nor
    indented by at least four spaces.
    """
    lines = compose_text.splitlines()
    header = f"  {name}:"
    if header not in lines:
        raise AssertionError(f"service {name!r} is not declared in docker-compose.yml")
    start = lines.index(header)
    block: list[str] = []
    for line in lines[start + 1:]:
        if line.strip() and not line.startswith("    "):
            break
        block.append(line)
    return "\n".join(block)


def command_tokens(raw: str) -> list[str]:
    """Split a Compose ``test:`` value into shell-ish tokens.

    Handles both the flow list (``[CMD-SHELL, 'a b c']``) and the block list
    form, and strips the quoting YAML needs so the caller can look at meaning
    rather than formatting.
    """
    raw = raw.strip()
    if raw.startswith("["):
        raw = raw[1:]
    if raw.endswith("]"):
        raw = raw[:-1]
    tokens: list[str] = []
    for chunk in raw.split(","):
        for piece in chunk.split():
            token = piece.strip().strip("'\"")
            if token:
                tokens.append(token)
    return tokens


def healthcheck_tokens(compose_text: str, service: str) -> list[str]:
    """Return the tokens of a service's ``healthcheck.test`` command.

    Accepts both YAML forms of the command: the flow list on the ``test:`` line
    itself, and a block list of subsequent, more-indented items.
    """
    block = service_block(compose_text, service).splitlines()
    for index, line in enumerate(block):
        if not line.strip().startswith("test:"):
            continue
        inline = line.split("test:", 1)[1].strip()
        if inline:
            return command_tokens(inline)
        indent = len(line) - len(line.lstrip())
        tokens: list[str] = []
        for item in block[index + 1:]:
            if item.strip() and (len(item) - len(item.lstrip())) <= indent:
                break
            item = item.strip()
            if item.startswith("- "):
                item = item[2:]
            tokens.extend(command_tokens(item))
        return tokens
    raise AssertionError(f"service {service!r} declares no healthcheck test")


def option_value(tokens: list[str], names: tuple[str, ...]) -> str | None:
    """Return the value of the first of ``names`` (``-d x`` or ``--dbname=x``)."""
    for index, token in enumerate(tokens):
        for name in names:
            if token == name and index + 1 < len(tokens):
                return tokens[index + 1]
            if token.startswith(f"{name}="):
                return token.split("=", 1)[1]
    return None


class PostgresReadinessProbeTest(unittest.TestCase):
    """The healthcheck must not be answerable by the temporary init server."""

    def test_postgres_healthcheck_probes_tcp_instead_of_the_unix_socket(self):
        tokens = healthcheck_tokens(read(COMPOSE), "postgres")
        self.assertIn(
            "pg_isready",
            tokens,
            "the postgres healthcheck must keep using pg_isready",
        )
        host = option_value(tokens, ("-h", "--host"))
        if host is None:
            self.fail(
                "pg_isready without -h probes the Unix socket, which the entrypoint's "
                "temporary server (listen_addresses='') answers before the real server "
                "listens on TCP; the healthcheck must name a TCP host"
            )
        self.assertFalse(
            host.startswith("/"),
            f"healthcheck host {host!r} is a Unix-socket directory, not TCP",
        )
        self.assertEqual(
            option_value(tokens, ("-p", "--port")),
            "5432",
            "the healthcheck must probe the port the services connect to",
        )
        self.assertEqual(
            option_value(tokens, ("-d", "--dbname")),
            "postgres",
            "the healthcheck must probe the always-present 'postgres' database",
        )

    def test_init_helper_is_mounted_into_the_database_initialisation_directory(self):
        block = service_block(read(COMPOSE), "postgres")
        self.assertIn(
            "/docker-entrypoint-initdb.d/",
            block,
            "postgres-init.sh must be mounted in /docker-entrypoint-initdb.d so the "
            "entrypoint runs it against the temporary server before the real one listens",
        )
        self.assertIn("postgres-init.sh", block)


class DatabaseCreationContractTest(unittest.TestCase):
    """Every database Compose connects to must be created by the init helper."""

    def test_every_compose_database_is_created_by_the_init_helper(self):
        compose_text = read(COMPOSE)
        referenced = set(re.findall(r"postgres://[^/\s]+/([A-Za-z0-9_]+)", compose_text))
        self.assertTrue(referenced, "expected at least one postgres:// URL in docker-compose.yml")
        created = set(re.findall(r"^\s*CREATE DATABASE\s+([A-Za-z0-9_]+)", read(POSTGRES_INIT), re.MULTILINE))
        missing = sorted(referenced - created)
        self.assertEqual(
            missing,
            [],
            "these databases are connected to but never created by postgres-init.sh: "
            f"{missing}; the readiness gate only guarantees databases that the init "
            "helper creates before the real server starts",
        )

    def test_test_databases_keep_their_application_roles(self):
        init_text = read(POSTGRES_INIT)
        for database, role in (("jobs_test", "jobs_app"), ("risk_test", "risk_app")):
            self.assertIn(f"CREATE DATABASE {database}", init_text)
            self.assertRegex(
                init_text,
                rf"GRANT CONNECT ON DATABASE {database} TO {role}",
                f"{database} must stay connectable by {role}, the role the suite uses",
            )


class TestProfileDependencyTest(unittest.TestCase):
    """The test-profile gate must keep waiting for migrated test databases."""

    def test_test_services_still_gate_on_their_completed_migrator(self):
        compose_text = read(COMPOSE)
        for service, migrator in TEST_PROFILE_GATES.items():
            block = service_block(compose_text, service)
            self.assertIn(
                migrator,
                block,
                f"{service} must keep depending on {migrator}",
            )
            self.assertIn(
                "service_completed_successfully",
                block,
                f"{service} must keep waiting for {migrator} to complete successfully",
            )

    def test_test_migrators_depend_on_postgres_health(self):
        compose_text = read(COMPOSE)
        for migrator in TEST_PROFILE_MIGRATORS:
            block = service_block(compose_text, migrator)
            self.assertIn("postgres:", block, f"{migrator} must keep depending on postgres")
            self.assertIn(
                "service_healthy",
                block,
                f"{migrator} depends on the postgres healthcheck; it must not start "
                "before the server is ready",
            )


class IntegrationWorkflowGateTest(unittest.TestCase):
    """The CI gate must stay strict and keep running the readiness checks."""

    def test_workflow_keeps_the_test_profile_gate_without_continue_on_error(self):
        workflow = read(INTEGRATION_WORKFLOW)
        for command in GATE_COMMANDS:
            self.assertIn(command, workflow, f"the workflow must keep running: {command}")
        self.assertNotIn(
            "continue-on-error",
            workflow,
            "a failing migration must fail the job; continue-on-error would hide it",
        )

    def test_workflow_runs_the_readiness_checks(self):
        workflow = read(INTEGRATION_WORKFLOW)
        for check in READINESS_CHECKS:
            self.assertIn(check, workflow, f"the workflow must keep running: {check}")

    def test_workflow_collects_diagnostics_before_teardown(self):
        workflow = read(INTEGRATION_WORKFLOW)
        diagnostics = workflow.find("logs --no-color")
        teardown = workflow.find("down -v")
        self.assertNotEqual(diagnostics, -1, "a failure must print container logs")
        self.assertNotEqual(teardown, -1, "the ephemeral data must still be torn down")
        self.assertLess(
            diagnostics,
            teardown,
            "container logs must be captured before the teardown removes the project",
        )


# ---------------------------------------------------------------------------
# Runtime harness contracts, driven by a fake Docker CLI.
#
# The fake is the only ``docker`` on the child's PATH and records every
# invocation (argv, cwd and any inherited COMPOSE_* keys) as one JSON line. It
# models a Compose project as a small state file so that partial creation,
# failed teardown and foreign labels can each be exercised deterministically
# without a Docker daemon.
# ---------------------------------------------------------------------------
FAKE_DOCKER = r'''\
import json
import os
import sys
import time

LOG = os.environ["FAKE_DOCKER_LOG"]
STATE = os.environ["FAKE_DOCKER_STATE"]
SCENARIO = set(filter(None, os.environ.get("FAKE_DOCKER_SCENARIO", "").split(",")))
SLEEP = float(os.environ.get("FAKE_DOCKER_SLEEP", "3"))
ARGV = sys.argv[1:]

with open(LOG, "a", encoding="utf-8") as handle:
    handle.write(json.dumps({
        "argv": ARGV,
        "cwd": os.getcwd(),
        "compose_env": {key: value for key, value in os.environ.items() if key.startswith("COMPOSE_")},
    }) + "\n")


def read_state():
    if not os.path.exists(STATE):
        return []
    with open(STATE, encoding="utf-8") as handle:
        return [line.strip() for line in handle if line.strip()]


def write_state(items):
    with open(STATE, "w", encoding="utf-8") as handle:
        handle.write("".join(item + "\n" for item in items))


def flag(name):
    for index, token in enumerate(ARGV):
        if token == name and index + 1 < len(ARGV):
            return ARGV[index + 1]
        if token.startswith(name + "="):
            return token.split("=", 1)[1]
    return None


PROJECT = flag("--project-name") or flag("-p") or "unknown"


def label():
    for item in read_state():
        if item.startswith("project:"):
            return item.split(":", 1)[1]
    return PROJECT


def listed(kind):
    return [item.split(":", 1)[1] for item in read_state() if item.startswith(kind + ":")]


def drop(kind, resource):
    write_state([item for item in read_state() if item != kind + ":" + resource])


def ok(text="fake: ok"):
    sys.stdout.write(text + "\n")
    sys.exit(0)


def bad(text):
    sys.stderr.write(text + "\n")
    sys.exit(1)


def skip_global_flags(rest):
    index = 0
    while index < len(rest):
        token = rest[index]
        if token in ("--project-directory", "--env-file", "--project-name", "--profile", "-p", "-f"):
            index += 2
            continue
        if token.startswith("-"):
            index += 1
            continue
        break
    return rest[index:]


if ARGV and ARGV[0] == "compose":
    command = skip_global_flags(ARGV[1:])
    head = command[0] if command else ""
    if head == "up":
        write_state([
            "project:" + PROJECT,
            "container:cid-postgres",
            "network:net-firewatch",
            "volume:vol-postgres",
        ])
        if "up-fail" in SCENARIO:
            bad("fake: postgres failed to start")
        ok("fake: up")
    if head == "ps":
        ok(PROJECT + "-postgres-1")
    if head == "exec":
        detail = "1" if any(token == "psql" for token in command) else "accepting connections"
        if "echo-secret" in SCENARIO:
            detail = os.environ.get("POSTGRES_PASSWORD", "") + "\n" + detail
        if "exec-fail" in SCENARIO:
            bad(detail)
        ok(detail)
    if head == "run":
        if "run-fail" in SCENARIO:
            bad("fake: migrator failed")
        ok("fake: migrator ok")
    if head == "logs":
        ok("fake: container log")
    if head == "down":
        if "down-slow" in SCENARIO:
            time.sleep(SLEEP)
        if "down-fail" in SCENARIO:
            bad("fake: down failed")
        write_state([item for item in read_state() if item.startswith("project:")])
        ok("fake: down")
    ok()

head = ARGV[0] if ARGV else ""


def labels_json():
    ok(json.dumps({
        "com.docker.compose.project": "someone-elses-project" if "foreign-label" in SCENARIO else label(),
    }))


if head == "ps":
    sys.stdout.write("".join(item + "\n" for item in listed("container")))
    sys.exit(0)
if head == "rm":
    for token in ARGV[2:]:
        drop("container", token)
    sys.exit(0)
if head == "volume":
    sub = ARGV[1] if len(ARGV) > 1 else ""
    if sub == "ls":
        sys.stdout.write("".join(item + "\n" for item in listed("volume")))
        sys.exit(0)
    if sub == "inspect":
        labels_json()
    if sub == "rm":
        drop("volume", ARGV[-1])
        sys.exit(0)
if head == "network":
    sub = ARGV[1] if len(ARGV) > 1 else ""
    if sub == "ls":
        sys.stdout.write("".join(item + "\n" for item in listed("network")))
        sys.exit(0)
    if sub == "inspect":
        labels_json()
    if sub == "rm":
        drop("network", ARGV[-1])
        sys.exit(0)
if head == "inspect":
    fmt = flag("-f") or ""
    if "Health" in fmt:
        if "inspect-slow" in SCENARIO:
            time.sleep(SLEEP)
        if "never-healthy" in SCENARIO:
            ok("starting")
        ok("unhealthy" if "unhealthy" in SCENARIO else "healthy")
    labels_json()
sys.exit(0)
'''

PROJECT_LABEL = "com.docker.compose.project"


class FakeDocker:
    """An isolated fake ``docker`` executable that records every invocation."""

    def __init__(self, root: Path):
        self.bin = root / "fakebin"
        self.bin.mkdir(parents=True)
        self.log = root / "docker-calls.jsonl"
        self.log.write_text("", encoding="utf-8")
        self.state = root / "docker-state.txt"
        self.state.write_text("", encoding="utf-8")
        self.child_tmp = root / "child-tmp"
        self.child_tmp.mkdir()
        (self.bin / "fake_docker.py").write_text(FAKE_DOCKER, encoding="utf-8")
        (self.bin / "docker.cmd").write_text(
            f'@echo off\r\n"{sys.executable}" "%~dp0fake_docker.py" %*\r\n', encoding="utf-8"
        )
        posix = self.bin / "docker"
        posix.write_text(
            f'#!/bin/sh\nexec "{sys.executable}" "$(dirname "$0")/fake_docker.py" "$@"\n', encoding="utf-8"
        )
        posix.chmod(0o755)
        self.exe = self.bin / ("docker.cmd" if os.name == "nt" else "docker")

    def assert_isolated(self) -> None:
        """Fail closed unless the owned fake is the only resolvable docker."""
        resolved = shutil.which("docker", path=str(self.bin))
        expected = os.path.normcase(os.path.abspath(str(self.exe)))
        if resolved is None or os.path.normcase(os.path.abspath(resolved)) != expected:
            raise AssertionError(f"fake docker is not isolated: resolved {resolved!r}")

    def run(self, scenario: str, *args: str, cwd: Path | None = None, sleep: str = "4", extra_env=None):
        self.assert_isolated()
        env = dict(os.environ)
        env["PATH"] = str(self.bin)
        env["FAKE_DOCKER_LOG"] = str(self.log)
        env["FAKE_DOCKER_STATE"] = str(self.state)
        env["FAKE_DOCKER_SCENARIO"] = scenario
        env["FAKE_DOCKER_SLEEP"] = sleep
        # The harness's temporary env file must land in this directory, never in backend/.
        env["TMPDIR"] = str(self.child_tmp)
        env.update(extra_env or {})
        return subprocess.run(
            [sys.executable, str(VERIFY_SCRIPT), *args],
            capture_output=True,
            text=True,
            env=env,
            cwd=str(cwd or ROOT),
            timeout=120,
        )

    def calls(self) -> list[dict]:
        text = self.log.read_text(encoding="utf-8")
        return [json.loads(line) for line in text.splitlines() if line.strip()]

    def leftovers(self) -> list[str]:
        return os.listdir(self.child_tmp)

    def output(self) -> str:
        return "".join(item.split(":", 1)[1] + "\n" for item in self.state.read_text(encoding="utf-8").splitlines())


def same_path(left: str, right: Path) -> bool:
    return os.path.normcase(os.path.abspath(left)) == os.path.normcase(os.path.abspath(str(right)))


def compose_calls(calls: list[dict]) -> list[dict]:
    return [call for call in calls if call["argv"][:1] == ["compose"]]


def flag_value(argv: list[str], name: str) -> str | None:
    if name in argv:
        index = argv.index(name)
        if index + 1 < len(argv):
            return argv[index + 1]
    return None


class ReadinessHarnessInvocationTest(unittest.TestCase):
    """Every docker command must be pinned, independent of the caller's context."""

    def test_every_command_pins_manifest_project_directory_and_cwd(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            fake = FakeDocker(root)
            elsewhere = root / "elsewhere"
            elsewhere.mkdir()
            result = fake.run(
                "ok",
                cwd=elsewhere,
                extra_env={
                    "COMPOSE_FILE": str(root / "hostile.yml"),
                    "COMPOSE_PROJECT_NAME": "hostile-project",
                    "COMPOSE_PROFILES": "hostile-profile",
                    "COMPOSE_ENV_FILES": str(root / "hostile.env"),
                },
            )
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            calls = fake.calls()
            self.assertTrue(calls, "the fake recorded no docker invocation")
            pinned = compose_calls(calls)
            self.assertTrue(pinned, "the harness never invoked docker compose")
            for call in calls:
                argv = call["argv"]
                self.assertTrue(
                    same_path(call["cwd"], ROOT),
                    f"a command ran outside the repository root: cwd={call['cwd']!r} argv={argv}",
                )
                self.assertEqual(
                    call["compose_env"], {}, f"inherited COMPOSE_* reached the command: {argv}"
                )
            for call in pinned:
                argv = call["argv"]
                manifest = flag_value(argv, "-f")
                directory = flag_value(argv, "--project-directory")
                env_file = flag_value(argv, "--env-file")
                project = flag_value(argv, "--project-name")
                if manifest is None:
                    self.fail(f"compose call without a pinned manifest: {argv}")
                if directory is None:
                    self.fail(f"compose call without a project directory: {argv}")
                if env_file is None:
                    self.fail(f"compose call without an env file: {argv}")
                if project is None:
                    self.fail(f"compose call without a pinned project name: {argv}")
                self.assertTrue(same_path(manifest, COMPOSE), f"wrong manifest: {manifest!r}")
                self.assertTrue(same_path(directory, ROOT), f"wrong project directory: {directory!r}")
                self.assertTrue(
                    same_path(os.path.dirname(env_file), fake.child_tmp),
                    f"the harness did not use its own temporary env file: {env_file!r}",
                )
                self.assertTrue(project.startswith("firewatch-ci-migration-verify-"))
                self.assertNotIn("hostile-project", argv)
                self.assertNotIn(str(ROOT / ".env"), argv)
            self.assertEqual(fake.leftovers(), [], "the temporary env file was not removed")


class ReadinessHarnessCleanupTest(unittest.TestCase):
    """Ownership, budgets and exit codes of the harness's cleanup path."""

    def test_partial_start_failure_is_still_cleaned_up(self):
        with tempfile.TemporaryDirectory() as tmp:
            fake = FakeDocker(Path(tmp))
            result = fake.run("up-fail")
            self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
            self.assertIn("ERROR", result.stdout)
            calls = fake.calls()
            consulted = [
                call for call in calls if any(token.startswith(f"label={PROJECT_LABEL}=") for token in call["argv"])
            ]
            self.assertTrue(consulted, "the harness never listed resources by project label")
            down = [call for call in calls if "down" in call["argv"]]
            self.assertTrue(down, "a partially created project was left behind")
            self.assertEqual(fake.leftovers(), [], "the temporary env file was not removed")
            self.assertEqual(
                [line for line in fake.output().splitlines() if line.startswith(("container:", "network:", "volume:"))],
                [],
                "resources survived the cleanup of a partial start",
            )

    def test_cleanup_failure_exit_codes_keep_the_primary_result(self):
        with tempfile.TemporaryDirectory() as tmp:
            passed_fake = FakeDocker(Path(tmp) / "passed")
            passed = passed_fake.run("down-fail")
            self.assertEqual(passed.returncode, 3, passed.stdout + passed.stderr)
            self.assertIn("cleanup error", passed.stdout)
            self.assertEqual(
                [
                    line
                    for line in passed_fake.output().splitlines()
                    if line.startswith(("container:", "network:", "volume:"))
                ],
                [],
                "the verified-label fallback did not remove the owned resources",
            )
        with tempfile.TemporaryDirectory() as tmp:
            failed_fake = FakeDocker(Path(tmp))
            failed = failed_fake.run("down-fail,run-fail")
            self.assertEqual(failed.returncode, 1, failed.stdout + failed.stderr)
            self.assertIn("FAIL", failed.stdout)
            self.assertIn("cleanup error", failed.stdout)

    def test_foreign_labelled_resource_is_never_deleted(self):
        with tempfile.TemporaryDirectory() as tmp:
            fake = FakeDocker(Path(tmp))
            result = fake.run("down-fail,foreign-label")
            self.assertEqual(result.returncode, 3, result.stdout + result.stderr)
            calls = fake.calls()
            self.assertEqual([call for call in calls if call["argv"][:1] == ["rm"]], [])
            self.assertEqual([call for call in calls if call["argv"][:2] == ["volume", "rm"]], [])
            self.assertEqual([call for call in calls if call["argv"][:2] == ["network", "rm"]], [])
            self.assertIn("refusing", result.stdout)
            self.assertEqual(fake.leftovers(), [], "the temporary env file was not removed")

    def test_verification_timeout_still_runs_cleanup_with_its_own_budget(self):
        with tempfile.TemporaryDirectory() as tmp:
            fake = FakeDocker(Path(tmp))
            # No --cleanup-timeout here: cleanup must not inherit the exhausted
            # verification deadline that stopped the readiness poll, and the slow
            # teardown must still get its default budget.
            result = fake.run("never-healthy,down-slow", "--timeout", "3")
            self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
            calls = fake.calls()
            self.assertTrue(calls, "the fake recorded no docker invocation")
            inspects = [index for index, call in enumerate(calls) if call["argv"][:1] == ["inspect"]]
            downs = [index for index, call in enumerate(calls) if "down" in call["argv"]]
            self.assertTrue(inspects, "the readiness poll never inspected the container")
            self.assertTrue(
                downs, "cleanup did not run after the verification budget was exhausted"
            )
            self.assertGreater(
                downs[0], inspects[-1], "cleanup ran before the readiness poll stopped"
            )
            self.assertEqual(fake.leftovers(), [], "the temporary env file was not removed")

    def test_timed_out_command_is_not_retried(self):
        with tempfile.TemporaryDirectory() as tmp:
            fake = FakeDocker(Path(tmp))
            result = fake.run("inspect-slow", "--timeout", "6", sleep="12")
            self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
            inspects = [call for call in fake.calls() if call["argv"][:1] == ["inspect"]]
            self.assertEqual(len(inspects), 1, "a timed-out command was retried")
            self.assertEqual(fake.leftovers(), [], "the temporary env file was not removed")

    def test_cleanup_timeout_is_reported_and_the_env_file_is_still_removed(self):
        with tempfile.TemporaryDirectory() as tmp:
            fake = FakeDocker(Path(tmp))
            result = fake.run(
                "never-healthy,down-slow",
                "--timeout",
                "3",
                "--cleanup-timeout",
                "1",
            )
            self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
            self.assertIn("cleanup", result.stdout)
            self.assertIn("timed out", result.stdout)
            self.assertEqual(fake.leftovers(), [], "the temporary env file was not removed")

    def test_generated_secret_is_redacted_in_output(self):
        with tempfile.TemporaryDirectory() as tmp:
            fake = FakeDocker(Path(tmp))
            # The fake echoes the generated password; the failing check prints it.
            result = fake.run("echo-secret,exec-fail")
            self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
            combined = result.stdout + result.stderr
            self.assertIn("FAIL", combined)
            self.assertIn("<redacted>", combined)
            self.assertIsNone(re.search(r"\b[0-9a-f]{48}\b", combined), "a generated secret reached the output")


if __name__ == "__main__":
    unittest.main()
