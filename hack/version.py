#!/usr/bin/env python3

import git
import os
import re
import sys
from collections import defaultdict

# HIVE_VERSION_PREFIX is the prefix of the Hive version that will be constructed by
# this script if we default to tracking the master branch.
# The version used for images and bundles will be:
# "{prefix}.{number of commits}-{git hash}"
# e.g. "v1.2.3187-18827f6"
HIVE_VERSION_PREFIX = "1.2"

# Match things like 'mce-2.0' or origin/mce-2.0, capturing:
# 1: 'origin/'
# 2: '2.0' (used for the bundle semver prefix)
MCE_BRANCH_RE = re.compile("^([^/]+/)?mce-(\d+\.\d+)$")
MASTER_BRANCH_RE = re.compile("^([^/]+/)?master$")

HIVE_BRANCH_DEFAULT = "master"
CHANNEL_DEFAULT = "alpha"

_mode = "library"


def parse_branch_name(branch_name):
    """Split up a branch name of the form 'mce-X.Y.

    :param branch_name: A branch name. If of the form [remote/]mce-M.N we will parse
        it as noted below; otherwise the first return will be False.
    :return parsed (bool): True if the branch_name was parseable; False otherwise.
    :return remote (str): If parsed and the branch_name contained a remote/ prefix, it is
        returned here; otherwise this is the empty string.
    :return prefix (str): Two-digit semver prefix of the bundle to be generated. If the branch
        name is of the form [remote/]mce-X.Y, this will be X.Y;
        If not parseable, it will be the empty string.
    """

    m = MCE_BRANCH_RE.match(branch_name)
    if m:
        return True, m.group(1), m.group(2)
    m = MASTER_BRANCH_RE.match(branch_name)
    if m:
        return True, m.group(1), HIVE_VERSION_PREFIX
    return False, "", ""


def origin_branch_info(hive_repo):
    """Find remote references in origin/ repository

    :param hive_repo: The git.Repo for the local hive clone.
    :return ret (dict): A dictionary of all remote refs that match the "master" and "mce-*" format.
    The key is the commit SHA for that ref and values are a set of prefixes corresponding to the SHA"""
    ret = defaultdict(set)
    for ref in hive_repo.refs:
        parsed, remote, prefix = parse_branch_name(ref.name)
        # Only pay attention to remote refs
        if not parsed:
            continue
        if remote != "origin/":
            continue
        sha = hive_repo.rev_parse(ref.name).hexsha
        ret[sha].add(prefix)
    return ret


def is_origin_branch(hive_repo, commit_hash):
    # Does a remote master or mce-* branch exist
    return commit_hash in origin_branch_info(hive_repo)


def find_branch(hive_repo, branch_arg):

    # The user may have specified a branch name for which we don't have a local ref
    # (e.g. `mce-2.1`). Rather than forcing them to specify a qualified ref, we'll
    # automatically look for it in the `origin` and `upstream` remotes as well.
    for remote in ("", "origin/", "upstream/"):
        branch = remote + branch_arg
        log("Trying to find {} at {}".format(branch_arg, branch))
        try:
            commit = hive_repo.rev_parse(branch)
        except git.BadName:
            log("Couldn't find {} at `{}`".format(branch_arg, branch))
        else:
            log("Found {} at `{}`".format(branch_arg, branch))
            break
    else:
        log("Couldn't find {} branch!".format(branch_arg))
        sys.exit(1)
    return commit


def process_master_branch(hive_repo, branch_arg):

    # This will raise an exception if there's no such commit-ish
    commit = hive_repo.rev_parse(branch_arg)
    master_commit = find_branch(hive_repo, HIVE_BRANCH_DEFAULT)

    # This includes when the commit is equal to the master commit
    if not hive_repo.is_ancestor(commit, master_commit):
        log(
            "Commit {} is not equal to or an ancestor of the master branch".format(
                commit.hexsha
            )
        )
        sys.exit(1)
    return commit.hexsha, HIVE_VERSION_PREFIX


def process_current_branch(hive_repo):
    """Validate and process the current HEAD.

    :param hive_repo: The git.Repo for the local hive clone.
    :return commit_hash: The canonical full-length sha of the commit corresponding to the current HEAD.
    :return prefix: The two-digit semver prefix of the bundle version to use. If HEAD is a
        (a descendant of) master, we'll use HIVE_VERSION_PREFIX. If not, and it names an 'mce-X.Y'
        branch, we'll use X.Y. Otherwise `None` is returned.
    :raise: If we get a HEAD that's invalid (doesn't correspond to a real commit in hive_repo),
        or that's not 'mce-X.Y' or master.
    """
    # This will raise an exception if there's no such commit-ish
    commit = hive_repo.rev_parse("HEAD")

    prefix = None
    maybe_prefix = origin_branch_info(hive_repo).get(commit.hexsha, {})

    if len(maybe_prefix) > 1:
        try:
            branch_name = repo.active_branch.name
        except TypeError:
            pass
        else:
            _, _, prefix = parse_branch_name(branch_name)
        if prefix not in maybe_prefix:
            raise ValueError(
                "Ambigious value {}: Multiple prefix values possibe: {}'".format(
                    commit, maybe_prefix
                )
            )
    elif len(maybe_prefix) == 1:
        prefix = list(maybe_prefix)[0]

        # In standalone mode, we'll have the default
        # (HIVE_VERSION_PREFIX) if commit_hash is at the tip of the master. If we're on a
        # commit that's the tip of an origin/mce-* branch, we'll have a suitable prefix
        # and won't have gotten here. Otherwise -- e.g. if we're on a descendant commit of an
        # mce-* branch -- we'll return an empty prefix, and the caller can decide what to put in
        # there.
        # TODO: Support descendants of mce-* branches using the appropriate prefix. But that's
        # tricky because we have to handle the case where I'm descended from multiple different
        # branches (e.g. right after we fork an mce-*).
    return commit.hexsha, prefix


# gen_hive_version generates and returns the hive version eg. "1.2.3187-18827f6"
def gen_hive_version(repo, commit_hash, prefix):
    # sha is the first 7 characters of commit_hash
    sha = commit_hash[0:7]

    # num commits is the number of git commits counted from the first commit to the provided commit_hash
    # this is the equivalent of running:
    # git rev-list `git rev-list --parents HEAD | egrep "^[a-f0-9]{40}$"`..HEAD --count
    parent = repo.git.rev_list("--max-parents=0", commit_hash)
    num_commits = repo.git.rev_list(
        "--count",
        "{parent}..{commit_hash}".format(parent=parent, commit_hash=commit_hash),
    )

    return "{prefix}.{commits}-{sha}".format(
        prefix=prefix, commits=num_commits, sha=sha
    )


def log(*args, **kwargs):
    if _mode == "standalone":
        print(*args, file=sys.stderr, **kwargs)
    else:
        print(*args, **kwargs)


if __name__ == "__main__":
    _mode = "standalone"
    # This is a bit sloppy, but it'll do for now.
    repo_dir = os.path.dirname(os.path.dirname(os.path.realpath(__file__)))
    repo = git.Repo(repo_dir)
    commit, prefix = process_current_branch(repo)
    if not prefix:
        prefix = "UnknownBranch"
    print(gen_hive_version(repo, commit, prefix))
