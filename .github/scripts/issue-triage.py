#!/usr/bin/env python3
import os
import sys

import yaml
from github import Auth, Github


def get_owners(repo, path="OWNERS"):
    """Fetch and parse a Kubernetes-style OWNERS file from the repository."""
    contents = repo.get_contents(path)
    owners = yaml.safe_load(contents.decoded_content)
    return {
        "approvers": owners.get("approvers", []),
        "reviewers": owners.get("reviewers", []),
    }


def main() -> int:
    token = os.getenv("GITHUB_TOKEN")
    issue_num = os.getenv("GH_ISSUE")
    repo_name = os.getenv("GITHUB_REPOSITORY")

    g = Github(auth=Auth.Token(token))
    repo = g.get_repo(repo_name)

    issue_number = int(issue_num)
    issue = repo.get_issue(issue_number)

    owners = get_owners(repo)
    print(f"Approvers: {owners['approvers']}")
    print(f"Reviewers: {owners['reviewers']}")

    return 0


if __name__ == "__main__":
    sys.exit(main())