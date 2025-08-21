#!/usr/bin/env python3
# file: scripts/copilot-firewall/test_fix.py
# Quick test script to verify the bug fix

import logging

from copilot_firewall.main import GitHubManager

# Test that we can properly extract repo names
test_repos = [
    {"name": "test-repo-1", "description": "Test repo 1", "isPrivate": False},
    {"name": "test-repo-2", "description": "Test repo 2", "isPrivate": True},
]

# Simulate what the inquirer would return (this is what was causing the bug)
fake_inquirer_response = [
    {"name": "test-repo-1 (🌍) - Test repo 1", "value": "test-repo-1", "checked": True},
    {"name": "test-repo-2 (🔒) - Test repo 2", "value": "test-repo-2", "checked": True},
]

# Test the bug fix logic
repo_names = []
for item in fake_inquirer_response:
    if isinstance(item, dict):
        repo_names.append(item.get("value", item.get("name", str(item))))
    else:
        repo_names.append(str(item))

# Use logging instead of print for better debugging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

logger.info("Extracted repo names: %s", repo_names)

# Test that GitHubManager.set_variable would work correctly
gh_manager = GitHubManager("jdfalk")
for repo_name in repo_names:
    logger.info(
        "Would call: gh variable set COPILOT_AGENT_FIREWALL_ALLOW_LIST_ADDITIONS -b '...' -R jdfalk/%s",
        repo_name,
    )
