#!/usr/bin/env python3
"""
restrict-claude.py — Update claude.yml and claude-code-review.yml across repos
to restrict Claude usage to @reh3376 only.

Usage:
    python3 scripts/restrict-claude.py reh3376/repo1 reh3376/repo2 ...

Requirements:
    - GitHub CLI (gh) installed and authenticated
    - PAT with 'repo' + 'workflow' scopes (gh auth login)
"""

import sys
import json
import base64
import subprocess
import textwrap


CLAUDE_YML_OLD = """\
    if: |
      (github.event_name == 'issue_comment' && contains(github.event.comment.body, '@claude')) ||
      (github.event_name == 'pull_request_review_comment' && contains(github.event.comment.body, '@claude')) ||
      (github.event_name == 'pull_request_review' && contains(github.event.review.body, '@claude')) ||
      (github.event_name == 'issues' && (contains(github.event.issue.body, '@claude') || contains(github.event.issue.title, '@claude')))\
"""

CLAUDE_YML_NEW = """\
    if: |
      (github.event_name == 'issue_comment' && contains(github.event.comment.body, '@claude') && github.event.comment.user.login == 'reh3376') ||
      (github.event_name == 'pull_request_review_comment' && contains(github.event.comment.body, '@claude') && github.event.comment.user.login == 'reh3376') ||
      (github.event_name == 'pull_request_review' && contains(github.event.review.body, '@claude') && github.event.review.user.login == 'reh3376') ||
      (github.event_name == 'issues' && (contains(github.event.issue.body, '@claude') || contains(github.event.issue.title, '@claude')) && github.event.issue.user.login == 'reh3376')\
"""

REVIEW_YML_OLD = """\
    # Optional: Filter by PR author
    # if: |
    #   github.event.pull_request.user.login == 'external-contributor' ||
    #   github.event.pull_request.user.login == 'new-developer' ||
    #   github.event.pull_request.author_association == 'FIRST_TIME_CONTRIBUTOR'\
"""

REVIEW_YML_NEW = "    if: github.event.pull_request.user.login == 'reh3376'"

COMMIT_MSG = "ci: restrict Claude to @reh3376 only"

FILES = {
    ".github/workflows/claude.yml": (CLAUDE_YML_OLD, CLAUDE_YML_NEW),
    ".github/workflows/claude-code-review.yml": (REVIEW_YML_OLD, REVIEW_YML_NEW),
}


def gh_api(endpoint, method="GET", fields=None):
    cmd = ["gh", "api", endpoint, "--method", method]
    if fields:
        for k, v in fields.items():
            cmd += ["-f", f"{k}={v}"]
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        raise RuntimeError(f"gh api {endpoint} failed:\n{result.stderr}")
    return json.loads(result.stdout) if result.stdout.strip() else {}


def update_file(repo, filepath, old_text, new_text):
    print(f"  Fetching {filepath} ...")
    data = gh_api(f"repos/{repo}/contents/{filepath}")
    sha = data["sha"]
    content = base64.b64decode(data["content"]).decode()

    if old_text not in content:
        print(f"  ⚠  Pattern not found in {filepath} — skipping (may already be updated or differ)")
        return False

    updated = content.replace(old_text, new_text, 1)
    encoded = base64.b64encode(updated.encode()).decode()

    gh_api(
        f"repos/{repo}/contents/{filepath}",
        method="PUT",
        fields={
            "message": COMMIT_MSG,
            "content": encoded,
            "sha": sha,
        },
    )
    print(f"  ✓  Updated {filepath}")
    return True


def process_repo(repo):
    print(f"\n{'='*60}")
    print(f"Repo: {repo}")
    changed = 0
    for filepath, (old_text, new_text) in FILES.items():
        try:
            if update_file(repo, filepath, old_text, new_text):
                changed += 1
        except RuntimeError as e:
            print(f"  ✗  Error on {filepath}: {e}")
    print(f"  {changed}/{len(FILES)} files updated in {repo}")


def main():
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(1)

    repos = sys.argv[1:]
    for repo in repos:
        process_repo(repo)

    print("\nDone.")


if __name__ == "__main__":
    main()
