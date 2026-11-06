"""The downloader: verify before moving into place, and never overwrite silently.

No test reaches the network: the downloader takes its opener as a parameter, so the
whole path (temporary file, checksum, move) runs against an in-memory body.
"""

from __future__ import annotations

import hashlib
import io
import urllib.error

import pytest

import fetch_sample

BODY = b"Region,Provincia,Comuna,Inicio\nMaule,Talca,Talca,07/09/2018 09:38\n"
DIGEST = hashlib.sha256(BODY).hexdigest()


def opener_returning(body: bytes = BODY):
    """A stand-in for urllib that answers with a fixed body."""

    def opener(url, timeout):  # noqa: ARG001
        return io.BytesIO(body)

    return opener


def opener_refusing(url, timeout):  # noqa: ARG001
    raise urllib.error.URLError("no route to host")


def expected(monkeypatch, body: bytes = BODY):
    """Point the documented checksum at the small body the tests use."""
    monkeypatch.setattr(fetch_sample, "SOURCE_SHA256", hashlib.sha256(body).hexdigest())
    monkeypatch.setattr(fetch_sample, "SOURCE_BYTES", len(body))


def test_a_new_download_is_verified_and_moved_into_place(tmp_path, monkeypatch):
    expected(monkeypatch)
    destination = tmp_path / "data" / "sample.csv"
    fetched = fetch_sample.fetch(destination, opener=opener_returning())
    assert fetched.sha256 == DIGEST
    assert fetched.bytes == len(BODY)
    assert destination.read_bytes() == BODY
    assert not destination.with_name(destination.name + ".part").exists()


def test_a_checksum_mismatch_leaves_no_file_at_the_destination(tmp_path, monkeypatch):
    monkeypatch.setattr(fetch_sample, "SOURCE_SHA256", "0" * 64)
    monkeypatch.setattr(fetch_sample, "SOURCE_BYTES", len(BODY))
    destination = tmp_path / "sample.csv"
    with pytest.raises(fetch_sample.FetchError) as refusal:
        fetch_sample.fetch(destination, opener=opener_returning())
    assert "sha256" in str(refusal.value)
    assert not destination.exists()
    # The unverified bytes are kept, and named, rather than silently deleted.
    assert (tmp_path / "sample.csv.part").exists()


def test_a_wrong_byte_count_is_reported_before_the_checksum(tmp_path, monkeypatch):
    monkeypatch.setattr(fetch_sample, "SOURCE_SHA256", DIGEST)
    monkeypatch.setattr(fetch_sample, "SOURCE_BYTES", len(BODY) + 10)
    destination = tmp_path / "sample.csv"
    with pytest.raises(fetch_sample.FetchError) as refusal:
        fetch_sample.fetch(destination, opener=opener_returning())
    assert "bytes" in str(refusal.value)


def test_an_existing_verified_copy_is_reused_without_downloading(tmp_path, monkeypatch):
    expected(monkeypatch)
    destination = tmp_path / "sample.csv"
    destination.write_bytes(BODY)
    calls: list[str] = []

    def opener(url, timeout):  # noqa: ARG001
        calls.append(url)
        return io.BytesIO(b"should not be downloaded")

    fetched = fetch_sample.fetch(destination, opener=opener)
    assert fetched.sha256 == DIGEST
    assert calls == []
    assert destination.read_bytes() == BODY


def test_an_existing_file_is_never_overwritten_without_force(tmp_path, monkeypatch):
    expected(monkeypatch)
    destination = tmp_path / "sample.csv"
    destination.write_bytes(b"something else entirely")
    with pytest.raises(fetch_sample.FetchError) as refusal:
        fetch_sample.fetch(destination, opener=opener_returning())
    assert "not the documented version of the sample" in str(refusal.value)
    assert destination.read_bytes() == b"something else entirely"


def test_force_replaces_an_existing_file(tmp_path, monkeypatch):
    expected(monkeypatch)
    destination = tmp_path / "sample.csv"
    destination.write_bytes(b"something else entirely")
    fetched = fetch_sample.fetch(destination, force=True, opener=opener_returning())
    assert fetched.sha256 == DIGEST
    assert destination.read_bytes() == BODY


def test_a_transport_failure_creates_nothing(tmp_path):
    destination = tmp_path / "sample.csv"
    with pytest.raises(fetch_sample.FetchError) as refusal:
        fetch_sample.fetch(destination, opener=opener_refusing)
    assert "cannot download" in str(refusal.value)
    assert not destination.exists()


def test_the_notice_states_the_licence_the_citation_and_what_the_file_is_not():
    text = "\n".join(fetch_sample.notice())
    assert "CC BY 4.0" in text
    assert "Vidal-Silva" in text
    assert "10.71578/UXAUN5" in text
    assert "NOT a training set" in text
    assert fetch_sample.SOURCE_SHA256 in text


def test_print_license_needs_no_download(capsys):
    code = fetch_sample.main(["--print-license"])
    assert code == 0
    printed = capsys.readouterr().out
    assert "CC BY 4.0" in printed
    assert "doi.org/..." in printed  # the source's own truncated DOI, reproduced verbatim


def test_a_verified_download_is_reported_with_its_checksum(tmp_path, monkeypatch, capsys):
    expected(monkeypatch)
    destination = tmp_path / "sample.csv"
    code = fetch_sample.main(["--dest", str(destination)], opener=opener_returning())
    assert code == 0
    assert f"verified: sha256 {DIGEST}" in capsys.readouterr().out


def test_a_failed_download_exits_2_with_a_message(tmp_path, capsys):
    destination = tmp_path / "sample.csv"
    code = fetch_sample.main(["--dest", str(destination)], opener=opener_refusing)
    assert code == 2
    assert "cannot download" in capsys.readouterr().err
