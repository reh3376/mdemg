# Code of Conduct
## Purpose

**mdemg** is an engineering project. The goal is to ship correct, maintainable, secure software with minimal drama. This Code of Conduct defines how we collaborate so technical decisions stay technical, and people stay respected.

## Our Pledge

We pledge to make participation in this project harassment-free and professional for everyone, regardless of age, body size, disability, ethnicity, sex characteristics, gender identity and expression, level of experience, education, socio-economic status, nationality, personal appearance, race, religion, or sexual identity and orientation.

We’ll argue about ideas, not people.

## Expected Behavior (Engineering Norms)

### Be precise, not personal

- Critique code and reasoning, not the author.
- Prefer: “This fails when X happens because Y” over “This is bad.”
- If you’re guessing, label it as a hypothesis.

### Disagree like scientists

- Bring data: benchmarks, logs, minimal repro cases, measurements.
- If you propose a change, state: **goal**, **tradeoffs**, **alternatives considered**, **risk**.
- If someone disproves your claim, acknowledge it and update the plan.
- Own your mistakes, correct them quickly, and document what you've learned

### Keep discussions actionable

- Use issues for decisions and PRs for implementation.
- Avoid derailing threads; open a new issue if the scope changes.

### Reviews are about quality, not dominance

- Reviewers: be timely, specific, and propose solutions when possible.
- Authors: treat review as collaboration, not a verdict.
- If the review stalls, explicitly ask: “What would get this merged?”

### Respect time and attention (the scarcest resource)

- Use small PRs when possible.
- Include context in PR descriptions: what/why/how tested.
- Don’t dump 10,000 lines without justification or a migration plan.

## Unacceptable Behavior

- Harassment, threats, intimidation, or encouraging others to do so
- Discriminatory language, slurs, or demeaning jokes
- Sexualized content or unwanted sexual attention
- Personal attacks, name-calling, trolling, or repeated derailing
- Publishing private information without consent (doxxing, personal emails, private chats)
- Posting secrets (tokens/keys/passwords) or intentionally encouraging insecure practices
- Sustained disruption or “winning at all costs” behavior

## Engineering Safety & Security Rules

### Never commit secrets

**Do not** commit or paste:

- API keys (Hugging Face, OpenAI, GitHub tokens)
- SSH private keys
- `.env` files with secrets
- internal URLs/credentials
- proprietary datasets not meant for public release

If you accidentally commit a secret:

1. **Rotate the secret immediately.**
2. Remove it from the repo history if necessary (maintainers will guide).
3. Document what happened and how it’s prevented in the future.

### Log hygiene

- Logs should not include secrets or sensitive user data.
- Redact tokens and PII before posting logs to issues/PRs.
- Prefer minimal repro logs over full dumps.

### Vulnerability disclosure

If you discover a security issue:

- Do not open a public issue with exploit details.
- Contact the repo owner/maintainers privately via GitHub.
- Provide a clear description, impact assessment, and suggested remediation if possible.

## Contribution Etiquette (Practical Rules)

### Issues

A good issue includes:

- **Expected behavior**
- **Actual behavior**
- **Steps to reproduce**
- **Environment** (OS, Python version, relevant package versions)
- **Logs** (sanitized)

### Pull Requests

A good PR includes:

- **What changed** (summary)
- **Why** (problem statement / goal)
- **How tested** (commands, unit tests, benchmarks)
- **Risk** (edge cases, rollout/migration notes)
- **Screenshots** (if UI)

### Tests are part of the feature

- If you add functionality, add tests.
- If tests are not feasible, explain why and what mitigations exist.

### Performance changes require evidence

- If you claim “faster” or “less memory,” include numbers and how measured.
- Prefer before/after comparisons with the same inputs.

### Backwards compatibility and migrations

- If you break compatibility, call it out explicitly and provide a migration path.
- Avoid silent behavior changes; document them.

### Changelog mindset

- If a change affects users, document it.
- If the repo has release notes, update them.

## Scope

This Code of Conduct applies to:

- GitHub issues, pull requests, discussions, and code review
- Project documentation and community channels
- Any place where the project is represented

## Enforcement

Maintainers may take actions including:

1. Correction request (specific change requested)
2. Warning
3. Temporary restriction (limited participation)
4. Permanent ban
5. Escalation to GitHub moderation when necessary

We aim for proportional, transparent enforcement with minimal spectacle.

## Reporting

If you experience or witness unacceptable behavior:

- Public report: open a GitHub issue labeled **conduct** (only if safe/appropriate).
- Private report: contact the repository owner/maintainers via GitHub profile messaging.

Include:

- What happened (links/screenshots if relevant)
- Where/when it happened
- Who was involved
- Any relevant context

Reports will be handled as confidentially as possible.

## Attribution

Adapted from the Contributor Covenant (v2.1), tuned for engineering collaboration and security hygiene.
