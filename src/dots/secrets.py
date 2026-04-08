"""Age-based secret encryption and decryption."""

from __future__ import annotations

import shutil
import subprocess
from pathlib import Path

from dots.errors import DotsError
import dots.utils as _utils


def decrypt_file(src: Path, identity: Path) -> bytes:
    if not shutil.which("age"):
        raise DotsError(
            "Cannot decrypt {} — 'age' not found on PATH".format(src.name),
            hint="Install age first:\n  dots tools install age\n"
                 "Or download from: https://github.com/FiloSottile/age/releases",
        )
    if not identity.exists():
        raise DotsError(
            "Failed to decrypt {}".format(src.name),
            hint="Reason: age identity file not found: {}\n\n"
                 "Hint: Generate an age keypair with:\n"
                 "  age-keygen -o {}\n"
                 "Then set the public key as recipient in dots.toml:\n"
                 "  [secrets]\n"
                 "  recipient = \"age1...\"".format(identity, identity),
        )
    result = subprocess.run(
        ["age", "--decrypt", "-i", str(identity), str(src)],
        capture_output=True,
    )
    if result.returncode != 0:
        stderr = result.stderr.decode().strip() if result.stderr else "unknown error"
        raise DotsError(
            "Failed to decrypt {}".format(src.name),
            hint="Reason: {}\n\nCheck that the identity file matches the recipient.".format(stderr),
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
            hint="Set in dots.toml:\n  [secrets]\n  recipient = \"age1...\"",
        )
    _utils.run(["age", "--encrypt", "-r", recipient, "-o", str(output), str(src)])
