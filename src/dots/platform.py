"""Platform and architecture detection."""

from __future__ import annotations

import os
import platform
import socket

__all__ = [
    "detect_platform",
    "detect_arch",
    "detect_goarch",
    "detect_os_name",
    "get_hostname",
]


def detect_platform() -> str:
    if os.path.isdir("/data/data/com.termux"):
        return "termux"
    s = platform.system().lower()
    if s == "darwin":
        return "darwin"
    if s == "windows":
        return "windows"
    return "linux"


def detect_arch() -> str:
    machine = platform.machine().lower()
    mapping = {
        "x86_64": "x86_64",
        "amd64": "x86_64",
        "aarch64": "aarch64",
        "arm64": "aarch64",
        "armv7l": "armv7",
        "i686": "i686",
        "i386": "i686",
    }
    return mapping.get(machine, machine)


def detect_goarch() -> str:
    arch = detect_arch()
    mapping = {
        "x86_64": "amd64",
        "aarch64": "arm64",
        "i686": "386",
        "armv7": "armv6l",
    }
    return mapping.get(arch, arch)


def detect_os_name() -> str:
    p = detect_platform()
    if p in ("linux", "termux"):
        return "linux"
    return p


def get_hostname() -> str:
    return socket.gethostname()
