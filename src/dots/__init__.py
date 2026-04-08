"""dots — dotfile management, tool installation, and shell environment generation."""

from dots.cli import *
from dots.config import *
from dots.constants import *
from dots.deploy import *
from dots.discovery import *
from dots.errors import *
from dots.git import *
from dots.platform import *
from dots.presets import *
from dots.repos import *
from dots.secrets import decrypt_file as decrypt_file
from dots.secrets import encrypt_file as encrypt_file
from dots.shell import *
from dots.ssh import *
from dots.templates import *
from dots.tools import *
from dots.utils import *

# Re-export optional imports used by tests
try:
    import jinja2  # type: ignore[import-untyped]
except ImportError:
    jinja2 = None  # type: ignore[assignment]
