#!/usr/bin/env python3

"""
Wrapper around version2.sh to provide a Python interface.
All version calculation logic lives in version2.sh.
"""

import git
import os
import subprocess


def _get_master_branch_prefix():
    """Extract MASTER_BRANCH_PREFIX from version2.sh."""
    script_path = os.path.join(os.path.dirname(__file__), "version2.sh")

    result = subprocess.run(
        ['/bin/bash', '-c', 'source "$1" && echo "$MASTER_BRANCH_PREFIX"', '_', script_path],
        capture_output=True,
        text=True,
        check=True
    )

    prefix = result.stdout.strip()
    if not prefix:
        raise RuntimeError(
            f"Failed to extract MASTER_BRANCH_PREFIX from {script_path}\n"
            f"stderr: {result.stderr}"
        )
    return prefix


# Read from version2.sh to ensure single source of truth
MASTER_BRANCH_PREFIX = _get_master_branch_prefix()


class Version:
    """
    Version wrapper that calls version2.sh for all version calculations.
    """
    def __init__(self, repo_dir, branch_name=None, commit_ish=None):
        self.repo = git.Repo(repo_dir)

        # Determine the commit
        if commit_ish:
            self.commit = self.repo.commit(commit_ish)
        else:
            self.commit = self.repo.head.commit

        # Call version2.sh to compute version info
        self.semver, self.shortsha = self._call_version_script(branch_name, commit_ish)

    def _call_version_script(self, branch_name, commit_ish):
        """Call version2.sh to compute semver and shortsha."""
        script_path = os.path.join(os.path.dirname(__file__), "version2.sh")

        # Source version2.sh and call its functions
        bash_script = '''
source "$1"
version_init "$2" "$3"
echo "$(semver)"
echo "$(shortsha)"
'''

        result = subprocess.run(
            ['/bin/bash', '-c', bash_script, '_', script_path, branch_name or '', commit_ish or ''],
            cwd=self.repo.working_dir,
            capture_output=True,
            text=True,
            check=True
        )

        # Validate output format
        lines = [line.strip() for line in result.stdout.splitlines() if line.strip()]
        if len(lines) != 2:
            raise RuntimeError(
                f"Expected 2 lines from version2.sh, got {len(lines)}\n"
                f"stdout: {result.stdout}\n"
                f"stderr: {result.stderr}"
            )

        semver_val = lines[0]
        shortsha_val = lines[1]

        return semver_val, shortsha_val

    def __str__(self):
        """Return version string with 'v' prefix."""
        return f"v{self.semver}"
