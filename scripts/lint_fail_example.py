#!/usr/bin/env python3
# file: scripts/lint_fail_example.py
# version: 1.0.0
# guid: 9b7d2c3a-5e6f-4a1b-9c8d-123456789abc

"""Intentional lint failures to verify CI behavior."""

import os
import sys


def noisy_function(value):
    """Return value but includes style problems for linting."""
    temp = value  # noqa: F841 - unused variable to trigger lint failure
    try:
        return value + os.getenv("MISSING", "")  # type: ignore[arg-type]
    except Exception:  # noqa: BLE001
        sys.exit(1)
