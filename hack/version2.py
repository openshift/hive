#!/usr/bin/env python3

import enum
import os
import re
import sys

import git

# Match things like 'mce-2.0' or origin/mce-2.0, capturing:
# 1: 'origin/'
# 2: '2.0' (used for the bundle semver prefix)
MCE_BRANCH_RE = re.compile("^([^/]+/)?mce-(\\d+\\.\\d+)$")

MASTER_BRANCH_RE = re.compile("^([^/]+/)?master$")

# MASTER_BRANCH_PREFIX is the prefix of the Hive version that will be constructed by
# this script if we are tracking the master branch.
# The version used for images and bundles will be:
# "v{prefix}.{number of commits}-{git hash}"
# e.g. "v1.2.3187-18827f6"
MASTER_BRANCH_PREFIX = "1.2"

# UNKNOWN_BRANCH_PREFIX is the fallback if we can't determine an MCE or master branch
UNKNOWN_BRANCH_PREFIX = "0.0"


class Version:
    def __init__(self, repo_dir: str, branch_name: str = None, commit_ish: str = None):
        self.repo = git.Repo(repo_dir)

        if commit_ish:
            # Commit explicitly given
            self.commit = self.repo.rev_parse(commit_ish)
        else:
            # Assume HEAD. If we got branch_name but not commit_ish, and they're sitting somewhere unrelated,
            # that's a user error. (To build on the branch commit, just specify the branch to commit_ish.)
            self.commit = self.repo.head.commit

        self._branch_name = ""
        if branch_name:
            self._branch_name = self._validate_branch(branch_name)
        else:
            # Before looking for related refs:
            # If we have an active branch, and it's equal to our commit, use it.
            try:
                if self.repo.active_branch.commit == self.commit:
                    self._branch_name = self.repo.active_branch.name
                    log(
                        f"Using active branch {self._branch_name} since it corresponds to commit {self.shortsha}",
                        level="debug",
                    )
            except TypeError:
                log("No active branch -- detached HEAD?", level="debug")

            if not self._branch_name:
                log(
                    f"Trying to discover a suitable branch from commit {self.shortsha}...",
                    level="debug",
                )
                self._branch_name = self._branch_from_commit(commit_ish)

        self._prefix = self._prefix_from_branch()

        self._commit_count = self.repo.git.rev_list(
            "--count",
            "{parent}..{commit_hash}".format(
                parent=self.repo.git.rev_list("--max-parents=0", self.commit.hexsha),
                commit_hash=self.commit.hexsha,
            ),
        )

    def _validate_branch(self, branch_name):
        """Ensure the named branch is related to (at, an ancestor of, or descended from) our commit.

        :param branch_name (str): A branch name. As a convenience, if not fully qualified, and no such branch exists
            locally, we will try to find it at origin/ and upstream/.
        :return branch_name (str): The branch name.
        :error: If the named branch is not related to our commit.
        """
        branch_commit = self._find_branch(branch_name)
        if branch_commit == self.commit:
            log("Branch matches commit", level="debug")
            return branch_name
        if self.repo.is_ancestor(self.commit, branch_commit):
            log(
                "Commit is an ancestor of branch: you are requesting a version for an old commit!",
                level="debug",
            )
            return branch_name
        if self.repo.is_ancestor(branch_commit, self.commit):
            log(
                "Branch is an ancestor of commit: you are requesting a version for an unmerged commit!",
                level="debug",
            )
            return branch_name
        # Die
        log(
            f"Commit {self.shortsha} and branch {branch_name} ({branch_commit.hexsha[0:8]}) do not appear to be related!",
            level="fatal",
        )

    def _branch_from_commit(self, commit_ish: str):
        here = set()
        ancestors = set()
        descendants = set()
        # Sadly, this is kind of expensive.
        for ref in self.repo.references:
            # If we got an explicit commit_ish, and it's already a branch name, use it.
            # Exact match
            if ref.name == commit_ish:
                log(f"Found a branch for commit_ish {commit_ish}", level="debug")
                return commit_ish
            # Match minus remote name
            if ref.is_remote() and ref.remote_head == commit_ish:
                log(
                    f"Found a branch for commit_ish {commit_ish} at remote '{ref.remote_name}'",
                    level="debug",
                )
                return commit_ish

            refname = ref.remote_head if ref.is_remote() else ref.name
            if refname == "HEAD":
                continue

            # Okay, now start using self._commit, which is already rev_parse()d.
            # If this ref is at our commit, it's a candidate
            if ref.commit == self.commit:
                here.add(refname)
            elif self.repo.is_ancestor(ref.commit, self.commit):
                ancestors.add(refname)
            elif self.repo.is_ancestor(self.commit, ref.commit):
                descendants.add(refname)

        # First look for a unique branch right here
        if len(here) == 1:
            branch = list(here)[0]
            log(
                f"Found unique branch {branch} at commit {self.shortsha}", level="debug"
            )
            return branch
        if len(here) > 1:
            # We can't handle this
            log(
                f"Found {len(here)} branches at commit {self.shortsha}! Please specify a branch name explicitly.",
                level="fatal",
            )

        # Next look for named descendants of our commit. This indicates we're versioning an old commit on the branch.
        if len(descendants) == 1:
            branch = list(descendants)[0]
            log(
                f"Found branch {branch} descended from commit {self.shortsha}",
                level="debug",
            )
            return branch

        log(f"descendants: {descendants}", level="debug")

        if len(descendants) == 0:
            log(f"WARNING: Are you versioning an unmerged commit?", level="warning")

        if len(ancestors) == 1:
            branch = list(ancestors)[0]
            log(
                f"Found branch {branch} which is an ancestor of commit {self.shortsha}",
                level="debug",
            )
            return branch

        log(
            f"Could not determine a branch! Please specify one explicitly.",
            level="fatal",
        )

    @property
    def shortsha(self):
        return self.commit.hexsha[0:7]

    def _prefix_from_branch(self):
        """Determine the X.Y semver prefix for the branch name.

        :return prefix (str): Two-digit semver prefix corresponding to the branch.
            - If the branch name is of the form [remote/]mce-X.Y, this will be X.Y.
            - If the branch name is [remote/]master, we'll use the master prefix.
            - Otherwise, we'll use the "unknown" default.
        """
        m = MCE_BRANCH_RE.match(self._branch_name)
        if m:
            return m.group(2)

        m = MASTER_BRANCH_RE.match(self._branch_name)
        if m:
            return MASTER_BRANCH_PREFIX

        return UNKNOWN_BRANCH_PREFIX

    def _find_branch(self, branch_name: str):
        # The user may have specified a branch name for which we don't have a local ref
        # (e.g. `mce-2.1`). Rather than forcing them to specify a qualified ref, we'll
        # automatically look for it in the `upstream` and `origin` remotes as well.
        for remote in ("", "upstream/", "origin/"):
            branch = remote + branch_name
            log(f"Trying to find {branch_name} at {branch}")
            try:
                commit = self.repo.rev_parse(branch)
            except git.BadName:
                log(f"Couldn't find {branch_name} at `{branch}`")
            else:
                log(f"Found {branch_name} at `{branch}`")
                break
        else:
            log(f"Couldn't find {branch_name} branch!", level="fatal")
        return commit

    @property
    def semver(self):
        return f"{self._prefix}.{self._commit_count}-{self.shortsha}"

    def __str__(self) -> str:
        return f"v{self.semver}"


def log(*args, level="info", **kwargs):
    if _mode == "standalone":
        print(*args, file=sys.stderr, **kwargs)
    else:
        print(*args, **kwargs)
    if level == "fatal":
        sys.exit(1)


_mode = "library"

if __name__ == "__main__":
    _mode = "standalone"
    repo = os.path.dirname(os.path.dirname(os.path.realpath(__file__)))
    print(str(Version(repo)))
