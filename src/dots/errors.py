"""Error classes for dots."""

from __future__ import annotations


class DotsError(Exception):
    """User-facing error with structured message."""

    def __init__(self, message: str, hint: str = ""):
        self.hint = hint
        super().__init__(message)

    def render(self):
        lines = ["", "✗ " + str(self)]
        if self.hint:
            lines.append("")
            for line in self.hint.splitlines():
                lines.append("  " + line)
        return "\n".join(lines)


class ConfigError(DotsError):
    """Configuration parse or validation error."""

    pass


class DeployError(DotsError):
    """File deployment error."""

    pass


class ToolInstallError(DotsError):
    """Tool installation error."""

    pass
