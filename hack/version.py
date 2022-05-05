#!/usr/bin/env python3

import git
import os
import re
import sys

# HIVE_VERSION_PREFIX is the prefix of the Hive version that will be constructed by
# this script if we get (or default to) a --branch that tracks master. Otherwise we
# will use the branch to calculate the prefix (e.g. `--branch ocm-2.3` => '2.3').
# The version used for images and bundles will be:
# "{prefix}.{number of commits}-{git hash}"
# e.g. "v1.2.3187-18827f6"
HIVE_VERSION_PREFIX = "1.2"

# Match things like 'ocm-2.5-mce-2.0' or origin/ocm-2.5-mce-2.0, capturing:
# 1: 'origin/'
# 2: '2.5' (used for the bundle semver prefix)
# 3: 'mce-2.0' (used for the channel name)
MCE_BRANCH_RE = re.compile("^([^/]+/)?ocm-(\d+\.\d+)-(mce-\d+\.\d+)$")
# Match things like 'ocm-2.3' or 'origin/ocm-2.3', capturing:
# 1: 'origin/'
# 2: 'ocm-2.3'
# 3: '2.3'
OCM_BRANCH_RE = re.compile("^([^/]+/)?(ocm-(\d+\.\d+))$")

HIVE_BRANCH_DEFAULT = "master"
CHANNEL_DEFAULT = "alpha"

_mode = "library"


def parse_branch_name(branch_name):
    """Split up a branch name of the form 'ocm-X.Y[-mce-M.N].

    :param branch_name: A branch name. If of the form [remote/]ocm-X.Y[-mce-M.N] we will parse
        it as noted below; otherwise the first return will be False.
    :return parsed (bool): True if the branch_name was parseable; False otherwise.
    :return remote (str): If parsed and the branch_name contained a remote/ prefix, it is
        returned here; otherwise this is the empty string.
    :return prefix (str): Two-digit semver prefix of the bundle to be generated. If the branch
        name is of the form [remote/]ocm-X.Y, this will be X.Y; if of the form
        [remote/]ocm-X.Y-mce-M.N it will be M.N. If not parseable, it will be the empty string.
    :return channel (str): The name of the channel in which we'll include the bundle. If the
        branch name is of the form [remote/]ocm-X.Y, this will be ocm-X.Y; if of the form
        [remote/]ocm-X.Y-mce-M.N it will be mce-M.N. If not parseable, it will be the empty
        string.
    """
    m = MCE_BRANCH_RE.match(branch_name)
    if m:
        return True, m.group(1), m.group(2), m.group(3)
    m = OCM_BRANCH_RE.match(branch_name)
    if m:
        return True, m.group(1), m.group(3), m.group(2)
    return False, '', '', ''


def remote_branch_info(hive_repo):
    """"""
    ret = {}
    for ref in hive_repo.refs:
        parsed, remote, prefix, channel = parse_branch_name(ref.name)
        # Only pay attention to remote refs
        if not parsed:
            continue
        if remote != "origin/":
            continue
        ret[ref.name] = dict(
            prefix=prefix,
            channel=channel,
            commit_hash=hive_repo.rev_parse(ref.name).hexsha,
        )
    return ret


def is_ancestor(hive_repo, descendant, ancestor):
    return hive_repo.git.rev_list(
        "--ancestry-path", "{}..{}".format(ancestor, descendant)
    )


def process_branch(hive_repo, branch_arg):
    """Validate and process the input (or default) branch.

    :param hive_repo: The git.Repo for the local hive clone.
    :param branch_arg: The string commit-ish input (or defaulted) via the --branch arg.
    :return commit_hash: The canonical full-length sha of the commit corresponding to branch_arg.
    :return prefix: The two-digit semver prefix of the bundle version to use. If branch_arg is
        (a descendant of) master, we'll use HIVE_VERSION_PREFIX. If not, and it names an 'ocm-X.Y'
        branch, we'll use X.Y.
    :return channels: List of string channel names to update. If branch_arg is (a descendant of)
        master, this will include 'alpha'. If branch_arg names an 'ocm-X.Y' branch, it will include
        'ocm-X.Y'.
    :raise: If we get a branch_arg that's invalid (doesn't correspond to a real commit in hive_repo),
        or that's not 'ocm-X.Y' or (a descendant of) master.
    """
    # We're cloning the hive repo, so there's no local branch for ocm-X.Y[-mce-M.N]; but we don't want to
    # force the user to say 'origin/ocm-X.Y[-mce-M.N]'. Accommodate...
    parsed, remote, _, _ = parse_branch_name(branch_arg)
    if parsed and not remote:
        branch_arg = "origin/{}".format(branch_arg)

    # This will raise an exception if there's no such commit-ish
    commit_hash = hive_repo.rev_parse(branch_arg).hexsha

    # We need to know where master is, even if we're processing a different branch. However, if
    # we've cloned from something other than HIVE_REPO_DEFAULT, there may not be a local `master`
    # branch. So we may need to try origin and upstream to find it.
    for remote in ("", "origin/", "upstream/"):
        master_branch = remote + HIVE_BRANCH_DEFAULT
        log("Trying to find master at {}".format(master_branch))
        try:
            master_hash = hive_repo.rev_parse(master_branch).hexsha
        except git.BadName:
            log("Couldn't find master at `{}`".format(master_branch))
        else:
            log("Found master at `{}`".format(master_branch))
            break
    else:
        log("Couldn't find master branch!")
        sys.exit(1)

    is_master_ancestor = is_ancestor(hive_repo, commit_hash, master_hash)

    # Was (a commit on) an ocm-X.Y[-mce-M.N] branch specified?
    # This will find all such branch names and add them to the channel list.
    # However, we'll only support >1 entry in this list if we're tracking
    # master, because otherwise which would we use to compute the semver?
    channels = []
    maybe_prefix = None
    for info in remote_branch_info(hive_repo).values():
        if info["commit_hash"] == commit_hash:
            channels.append(info["channel"])
            # We'll only use this if we end up with one
            # entry in this list AND we're not tracking master
            maybe_prefix = info["prefix"]

    if commit_hash == master_hash or (is_master_ancestor and not maybe_prefix):
        prefix = HIVE_VERSION_PREFIX
        if commit_hash == master_hash or _mode == "standalone":
            channels.append(CHANNEL_DEFAULT)
    else:
        if len(channels) > 1:
            raise ValueError(
                "Found more than one channel candidate at {}: {}.\n".format(
                    branch_arg, channels
                )
                + "I can't handle this unless they're direct descendants of master -- "
                + "which one would I use as the semver base?"
            )
        prefix = maybe_prefix

    if not channels or not prefix:
        if _mode != "standalone":
            raise ValueError(
                "Can't make sense of branch {}: expected {}, a direct descendant thereof, 'ocm-X.Y', or 'ocm-X.Y-mce-M.N'.".format(
                    branch_arg, HIVE_BRANCH_DEFAULT
                )
            )
        # In standalone mode, we don't care about channels. For prefix, we'll have the default
        # (HIVE_VERSION_PREFIX) if commit_hash is at or descended from master. If we're on a
        # commit that's the tip of an origin/ocm* branch, we'll have suitable channels & prefix
        # and won't have gotten here. Otherwise -- e.g. if we're on a descendant commit of an
        # ocm* branch -- we'll return an empty prefix, and the caller can decide what to put in
        # there.
        # TODO: Support descendants of ocm* branches using the appropriate prefix. But that's
        # tricky because we have to handle the case where I'm descended from multiple different
        # branches (e.g. right after we fork an ocm*).
    return commit_hash, prefix, channels


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
    commit, prefix, _ = process_branch(repo, "HEAD")
    if not prefix:
        prefix = "UnknownBranch"
    print(gen_hive_version(repo, commit, prefix))
