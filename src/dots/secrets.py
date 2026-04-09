"""Age-based secret encryption and decryption."""

from __future__ import annotations

import shutil
import subprocess
from pathlib import Path

import dots.utils as _utils
from dots.errors import DotsError


def decrypt_file(src: Path, identity: Path) -> bytes:
    if not shutil.which("age"):
        raise DotsError(
            f"Cannot decrypt {src.name} — 'age' not found on PATH",
            hint="Install age first:\n  dots tools install age\n"
            "Or download from: https://github.com/FiloSottile/age/releases",
        )
    if not identity.exists():
        raise DotsError(
            f"Failed to decrypt {src.name}",
            hint=f"Reason: age identity file not found: {identity}\n\n"
            "Hint: Generate an age keypair with:\n"
            f"  age-keygen -o {identity}\n"
            "Then set the public key as recipient in dots.toml:\n"
            "  [secrets]\n"
            '  recipient = "age1..."',
        )
    result = subprocess.run(
        ["age", "--decrypt", "-i", str(identity), str(src)],
        capture_output=True,
    )
    if result.returncode != 0:
        stderr = result.stderr.decode().strip() if result.stderr else "unknown error"
        raise DotsError(
            f"Failed to decrypt {src.name}",
            hint=f"Reason: {stderr}\n\nCheck that the identity file matches the recipient.",
        )
    return result.stdout


def encrypt_file(src: Path, recipient: str, output: Path) -> None:
    if not shutil.which("age"):
        raise DotsError(
            "Cannot encrypt — 'age' not found on PATH",
            hint="Install age:\n  https://github.com/FiloSottile/age/releases",
        )
    if not recipient:
        raise DotsError(
            "No recipient configured for encryption",
            hint='Set in dots.toml:\n  [secrets]\n  recipient = "age1..."',
        )
    _utils.run(["age", "--encrypt", "-r", recipient, "-o", str(output), str(src)])
