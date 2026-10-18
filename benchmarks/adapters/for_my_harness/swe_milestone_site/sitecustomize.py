"""Registers the ABBD arm framework in every SWE-Milestone Python process that has ABBD_GROUP set.

run_all.py spawns one detached `python -m harness.e2e.run_e2e` per repo, so an in-process patch would
not reach the worker. PYTHONPATH pointing here does, and the harness checkout stays unedited. An import
failure is fatal on purpose: a group must never fall back to the stock agent without anyone noticing.
"""
import os
import sys

if os.environ.get("ABBD_GROUP"):
    sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
    import swe_milestone_agent  # noqa: F401,E402
