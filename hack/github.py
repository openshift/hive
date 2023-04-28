import json
import os
import requests


class GitHubClient:
    def __init__(self, user, repo, token=""):
        gh_token = ""

        if token != "":
            gh_token = token
        else:
            if os.environ.get("GITHUB_TOKEN") != None:
                gh_token = os.environ["GITHUB_TOKEN"]
            if os.environ.get("GH_TOKEN") != None:
                gh_token = os.environ["GH_TOKEN"]

        if gh_token == "":
            raise Exception("GitHub token not able to be set")

        self.user = user
        self.repo = repo
        self.headers = {
            "Authorization": "token " + gh_token,
            "Accept": "application/vnd.github.v3+json",
        }

    def _create_request(self, rest_path):
        return "https://api.github.com" + rest_path

    def create_annotated_tag(self, tag, tag_msg, commit_hash):
        data = {
            "tag": tag,
            "message": tag_msg,
            "object": commit_hash,
            "type": "commit",
        }

        create_tag_rest_path = "/repos/{}/{}/git/tags".format(self.user, self.repo)
        req = self._create_request(create_tag_rest_path)

        return requests.post(req, headers=self.headers, data=json.dumps(data))

    def create_reference(self, ref, sha):
        data = {
            "ref": ref,
            "sha": sha,
        }

        create_ref_rest_path = "/repos/{}/{}/git/refs".format(self.user, self.repo)
        req = self._create_request(create_ref_rest_path)

        return requests.post(req, headers=self.headers, data=json.dumps(data))

    def create_pr(self, pr_from, pr_to, pr_title, body):
        data = {
            "head": pr_from,
            "base": pr_to,
            "title": pr_title,
            "body": body,
        }

        create_pr_rest_path = "/repos/{}/{}/pulls".format(self.user, self.repo)
        req = self._create_request(create_pr_rest_path)
        return requests.post(req, headers=self.headers, data=json.dumps(data))
