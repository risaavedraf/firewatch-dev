"""Shared test setup: paths, and a guard that proves the suite is offline.

The suite has to run with no network (CI runs it on a runner with no source data,
and a test that downloads is a test that fails when GitHub is slow). The guard is
autouse and blocks the two ways package code could reach out, so "no network" is
enforced instead of merely intended.
"""

from __future__ import annotations

import socket
import urllib.request
from pathlib import Path

import pytest

TESTS_ROOT = Path(__file__).resolve().parent
FIXTURES = TESTS_ROOT / "fixtures"

#: The trimmed, real sample: 44 rows of the CONAF registry, 2018-11-20..2018-12-31,
#: comunas Talca and Cauquenes. See fixtures/PROVENANCE.md for the selection rule.
SAMPLE_FIXTURE = FIXTURES / "conaf-maule-sample.csv"

#: Fabricated rows for the two cases the real source cannot supply: a null date and
#: a burned area of zero. Clearly labelled as such in PROVENANCE.md.
SYNTHETIC_FIXTURE = FIXTURES / "synthetic-edge-cases.csv"


class _NoNetworkSocket(socket.socket):
    """A socket that refuses to connect, so no test can reach the network."""

    def connect(self, *args, **kwargs):  # noqa: ANN002, ANN003
        raise RuntimeError("the training test suite must not open a network connection")

    def connect_ex(self, *args, **kwargs):  # noqa: ANN002, ANN003
        raise RuntimeError("the training test suite must not open a network connection")


@pytest.fixture(autouse=True)
def no_network(monkeypatch):
    """Block outbound connections for every test in this directory."""

    def refuse(*args, **kwargs):  # noqa: ANN002, ANN003
        raise RuntimeError("the training test suite must not open a network connection")

    monkeypatch.setattr(socket, "socket", _NoNetworkSocket)
    monkeypatch.setattr(socket, "create_connection", refuse)
    monkeypatch.setattr(urllib.request, "urlopen", refuse)
