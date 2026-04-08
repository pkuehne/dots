"""dots — dotfile management, tool installation, and shell environment generation."""

from dots.constants import *
from dots.errors import *
from dots.platform import *
from dots.utils import *
from dots.config import *
from dots.discovery import *
from dots.templates import *
from dots.secrets import decrypt_file, encrypt_file
from dots.shell import *
from dots.git import *
from dots.ssh import *
from dots.deploy import *
from dots.tools import *
from dots.repos import *
from dots.presets import *
from dots.cli import *

# Re-export optional imports used by tests
try:
    import jinja2  # type: ignore[import-untyped]
except ImportError:
    jinja2 = None  # type: ignore[assignment]

try:
    from urllib.request import urlopen, Request
    from urllib.error import HTTPError, URLError
except ImportError:
    pass
