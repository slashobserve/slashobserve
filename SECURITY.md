# Security policy

## Report a vulnerability

Report a vulnerability privately through GitHub: open the repository's **Security** tab and choose **Report a vulnerability**, or go to https://github.com/slashobserve/slashobserve/security/advisories/new.
Do not open a public issue or pull request for it.

Include what you found, the version or commit, the steps to reproduce it, and what an attacker gains.
A proof of concept helps.
Do not include real credentials or data from a system you do not own.

The report stays private between you and the maintainers.
We confirm the problem, work out a fix, and publish an advisory with the fixed release.
We credit you in the advisory unless you ask us not to.

## Supported versions

SlashObserve is pre-1.0.
Only the latest release receives security fixes.

## Scope

In scope are the server, the `observe` CLI, the web app, the container image and the Compose files in this repository.
[Security model](docs/operate/security-model.md) describes what the server protects.

Out of scope:

- The demo stack's fixed logins, Postgres password and OIDC client secret.
  They are development values, and the demo binds to 127.0.0.1 for that reason.
  Running the demo on a network-reachable host is a misconfiguration, not a vulnerability.
- Findings that need an Admin to attack their own deployment, such as an Admin allowing a malicious domain on the outbound allowlist.
- Vulnerabilities in a dependency with no path to exploit them through SlashObserve; report those upstream.
