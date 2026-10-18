"""Exercise the real local stack. Tokens and passwords are never printed."""
import argparse
import base64
import json
from pathlib import Path
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

ROOT = Path(__file__).resolve().parent.parent
API = "http://localhost:8080"
IDENTITY = "http://localhost:8081/realms/firewatch/protocol/openid-connect/token"

# Canonical levels of the classification contract. This is asserted as membership
# rather than as one exact value on purpose: the level of the synthetic sample
# moves with the rule's parameters, so pinning it here would break on any
# legitimate calibration change while proving nothing extra.
CANONICAL_RISK_LEVELS = {"bajo", "medio", "alto", "desconocido"}


def credentials():
    return dict(line.split("=", 1) for line in (ROOT / ".env").read_text().splitlines()
                if line and not line.startswith("#"))


def request(url, method="GET", token=None, body=None, headers=None):
    headers = dict(headers or {})
    if token:
        headers["Authorization"] = "Bearer " + token
    if isinstance(body, dict):
        body = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=body, method=method, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=15) as response:
            data = response.read()
            return response.status, json.loads(data) if data else None
    except urllib.error.HTTPError as error:
        return error.code, None


def wait_for(check, timeout=120):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            value = check()
            if value:
                return value
        except (urllib.error.URLError, TimeoutError, ConnectionError):
            pass
        time.sleep(1)
    raise RuntimeError("Timed out waiting for the expected stack state")


def token_for(user, password):
    body = urllib.parse.urlencode({"grant_type": "password", "client_id": "firewatch-demo",
                                  "username": user, "password": password}).encode()
    def login():
        status, data = request(IDENTITY, "POST", body=body,
                               headers={"Content-Type": "application/x-www-form-urlencoded"})
        return data["access_token"] if status == 200 else None
    return wait_for(login)


def create_job(token, key=None):
    status, job = request(API + "/v1/jobs", "POST", token, {"zone_id": "valparaiso-demo"},
                          {"Idempotency-Key": key or str(uuid.uuid4())})
    assert status == 202, f"Create job returned {status}"
    return job


def await_job(token, job):
    def completed():
        status, result = request(API + "/v1/jobs/" + job["id"], token=token)
        return result if status == 200 and result["state"] == "published" else None
    return wait_for(completed)


def compose(*args):
    subprocess.run(["docker", "compose", *args], cwd=ROOT, check=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--resilience", action="store_true", help="Temporarily stop the local broker and restart app services")
    args = parser.parse_args()
    env = credentials()
    admin = token_for("demo-admin", env["DEMO_ADMIN_PASSWORD"])
    viewer = token_for("demo-viewer", env["DEMO_VIEWER_PASSWORD"])
    wait_for(lambda: request(API + "/health/ready")[0] == 204)
    assert request(API + "/v1/jobs")[0] == 401
    assert request(API + "/v1/jobs", "POST", viewer, {"zone_id": "valparaiso-demo"})[0] == 403
    assert request(API + "/v1/audit", token=viewer)[0] == 403
    assert request(API + "/v1/jobs", "POST", admin, {"zone_id": "unknown"},
                   {"Idempotency-Key": str(uuid.uuid4())})[0] == 400
    key = str(uuid.uuid4())
    job = create_job(admin, key)
    assert create_job(admin, key)["id"] == job["id"], "HTTP retry created a duplicate job"
    completed = await_job(admin, job)
    status, batch = request(API + "/v1/risk/latest?zone_id=valparaiso-demo", token=viewer)
    assert status == 200 and batch["id"] == completed["batch_id"]
    assert batch["demo"] is True and batch["risk_level"] in CANONICAL_RISK_LEVELS, \
        f"unexpected risk level {batch['risk_level']!r}"
    assert isinstance(batch.get("rationale"), str) and batch["rationale"], \
        "published batch carries no rationale"
    assert batch["geometry"]["type"] == "Point"
    status, history = request(API + "/v1/risk/history?zone_id=valparaiso-demo", token=viewer)
    assert status == 200 and sum(b["job_id"] == job["id"] for b in history) == 1
    status, audit = request(API + "/v1/audit", token=admin)
    assert status == 200
    assert {a["action"] for a in audit if a["job_id"] == job["id"]} == {"job.requested", "batch.published"}
    print("PASS: JWT/RBAC, input validation, idempotency, async publication, PostGIS, history and audit")

    if args.resilience:
        compose("stop", "rabbitmq")
        try:
            queued = create_job(admin)
            status, previous = request(API + "/v1/risk/latest?zone_id=valparaiso-demo", token=viewer)
            assert status == 200 and previous["id"] == batch["id"], "Last valid batch was lost"
            assert request(API + "/v1/jobs/" + queued["id"], token=admin)[1]["state"] == "queued"
        finally:
            compose("start", "rabbitmq")
        recovered = await_job(admin, queued)
        compose("restart", "jobs", "risk")
        # `latest` legitimately moves on across a restart: `risk` publishes its
        # first cycle batch for the zone as soon as it starts, so the batch this
        # step is about is asserted where the restart must have preserved it —
        # the history — while `latest` is asserted to answer one current batch.
        # Asserting the recovered id as `latest` would demand that no newer batch
        # exist, which the immediate startup cycle contradicts by design.
        def persisted():
            status, history = request(API + "/v1/risk/history?zone_id=valparaiso-demo", token=viewer)
            if status != 200 or not any(b["id"] == recovered["batch_id"] for b in history):
                return False
            status, current = request(API + "/v1/risk/latest?zone_id=valparaiso-demo", token=viewer)
            return status == 200 and current["data_status"] == "current"
        wait_for(persisted)
        print("PASS: broker outage, durable outbox recovery, last-valid-batch reads and app restart persistence")

    # Invalid messages must be quarantined without mutating the published batch.
    authorization = base64.b64encode(("firewatch:" + env["RABBITMQ_PASSWORD"]).encode()).decode()
    queue_url = "http://localhost:15672/api/queues/firewatch/risk.jobs.v1.dead"
    status, existing = request(queue_url, headers={"Authorization": "Basic " + authorization})
    assert status == 200
    previous_dead_letters = existing.get("messages_ready", 0)
    status, last_valid = request(API + "/v1/risk/latest?zone_id=valparaiso-demo", token=viewer)
    assert status == 200
    status, result = request("http://localhost:15672/api/exchanges/firewatch/firewatch.events/publish", "POST",
                            body={"properties": {"delivery_mode": 2}, "routing_key": "jobs.requested.v1",
                                  "payload": "{invalid", "payload_encoding": "string"},
                            headers={"Authorization": "Basic " + authorization})
    assert status == 200 and result["routed"]
    def quarantined():
        status, queue = request(queue_url,
                                headers={"Authorization": "Basic " + authorization})
        return status == 200 and queue.get("messages_ready", 0) > previous_dead_letters
    wait_for(quarantined, 30)
    status, after_invalid = request(API + "/v1/risk/latest?zone_id=valparaiso-demo", token=viewer)
    assert status == 200 and after_invalid["id"] == last_valid["id"]
    print("PASS: malformed event sent to dead-letter queue")
    print("Demo job:", job["id"])


if __name__ == "__main__":
    main()
