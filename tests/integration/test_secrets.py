"""Integration tests for encrypt/decrypt (age mocked)."""

from unittest.mock import MagicMock, patch

import pytest

from dots.errors import DotsError
from dots.secrets import decrypt_file, encrypt_file


def test_decrypt_calls_age(tmp_path):
    src = tmp_path / "secret.age"
    src.write_bytes(b"encrypted-content")
    identity = tmp_path / "key.txt"
    identity.write_text("AGE-SECRET-KEY-...")

    mock_result = MagicMock()
    mock_result.returncode = 0
    mock_result.stdout = b"decrypted-content"

    with (
        patch("shutil.which", return_value="/usr/bin/age"),
        patch("subprocess.run", return_value=mock_result) as mock_run,
    ):
        data = decrypt_file(src, identity)

    assert data == b"decrypted-content"
    cmd = mock_run.call_args[0][0]
    assert "age" in cmd
    assert "--decrypt" in cmd


def test_decrypt_no_age_binary(tmp_path):
    src = tmp_path / "secret.age"
    src.write_bytes(b"x")
    identity = tmp_path / "key.txt"
    identity.write_text("key")

    with patch("shutil.which", return_value=None):
        with pytest.raises(DotsError, match="age.*not found"):
            decrypt_file(src, identity)


def test_decrypt_no_identity(tmp_path):
    src = tmp_path / "secret.age"
    src.write_bytes(b"x")
    identity = tmp_path / "nonexistent-key.txt"

    with patch("shutil.which", return_value="/usr/bin/age"):
        with pytest.raises(DotsError, match="identity file not found|Failed to decrypt"):
            decrypt_file(src, identity)


def test_encrypt_calls_age(tmp_path):
    src = tmp_path / "secret"
    src.write_text("my secret")
    output = tmp_path / "secret.age"

    with patch("shutil.which", return_value="/usr/bin/age"), patch("dots.utils.run") as mock_run:
        encrypt_file(src, "age1abc...", output)

    cmd = mock_run.call_args[0][0]
    assert "age" in cmd
    assert "--encrypt" in cmd
    assert "-r" in cmd


def test_encrypt_no_recipient(tmp_path):
    src = tmp_path / "secret"
    src.write_text("x")
    output = tmp_path / "secret.age"

    with patch("shutil.which", return_value="/usr/bin/age"):
        with pytest.raises(DotsError, match="No recipient"):
            encrypt_file(src, "", output)


def test_encrypt_no_age(tmp_path):
    src = tmp_path / "secret"
    src.write_text("x")
    output = tmp_path / "secret.age"

    with patch("shutil.which", return_value=None):
        with pytest.raises(DotsError, match="age.*not found"):
            encrypt_file(src, "age1abc...", output)
