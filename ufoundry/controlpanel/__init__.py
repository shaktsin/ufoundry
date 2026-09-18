"""Local web control panel for UFoundry admin interface.

FastAPI and uvicorn are optional dependencies. Install with:
    pip install 'ufoundry[panel]'
"""

from __future__ import annotations

__all__ = ["create_app", "run_panel"]


def create_app(**kwargs):
    from ufoundry.controlpanel.server import create_app as _create  # noqa: PLC0415
    return _create(**kwargs)


def run_panel(**kwargs):
    from ufoundry.controlpanel.server import run_panel as _run  # noqa: PLC0415
    return _run(**kwargs)
