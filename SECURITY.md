# Security Policy

## Supported Versions

Deployik is developed in the open and shipped as a single, continuously
delivered control plane. Security fixes are applied to:

| Version            | Supported          |
| ------------------ | ------------------ |
| Latest `main`      | :white_check_mark: |
| Latest release tag | :white_check_mark: |
| Older releases     | :x:                |

We recommend self-hosters track the latest released image (or `main`) and keep
their deployment up to date, since fixes land on the tip first.

## Reporting a Vulnerability

**Please do not open a public GitHub issue for security problems.**

The preferred channel is **GitHub Private Vulnerability Reporting / Security
Advisories**:

> https://github.com/lefteq/lovinka-deployik/security/advisories/new

If you cannot use GitHub advisories, email the maintainer privately at
`security@<your-domain>` (replace with the maintainer's published address).

When reporting, please include:

- A description of the vulnerability and its impact.
- Steps to reproduce (proof-of-concept, affected endpoint/component, version or
  image tag).
- Any relevant logs, configuration, or environment details.

**Response targets:**

- We aim to **acknowledge** your report within **72 hours**.
- We will keep you updated as we investigate and work on a fix.
- Please give us a reasonable window to remediate before any public disclosure.
  Coordinated disclosure is appreciated, and we are happy to credit reporters.

## Security Model

Deployik is a **deployment control plane**: it clones tenant repositories,
builds Docker images, and runs the resulting containers on the host. Operators
must understand the trust boundaries before exposing it.

- **Docker socket = root-equivalent.** Deployik mounts the host Docker socket so
  it can build and run tenant containers. Anyone who can drive Deployik (or
  compromise it) can effectively run arbitrary code as **root on the host**.
  Treat the Deployik instance, its admin users, and its API tokens as
  root-equivalent. Run it on a host dedicated to this purpose, restrict who can
  authenticate, and keep it behind a trusted reverse proxy.
- **Arbitrary tenant builds.** Build steps execute code from connected GitHub
  repositories (install/build scripts, Dockerfiles). Only connect repositories
  you trust, and isolate the host accordingly.
- **Secrets encrypted at rest.** Environment variables, secrets, GitHub OAuth
  tokens, webhook secrets, and per-environment protection passwords are
  encrypted with AES-256-GCM (key derived from `ENCRYPTION_KEY`). Protect
  `ENCRYPTION_KEY` and `JWT_SECRET` — losing them compromises stored data and
  session integrity; rotating them invalidates existing encrypted data/sessions.
- **Token handling.** Authentication uses GitHub OAuth → short-lived JWT access
  tokens plus rotating refresh tokens (hashed at rest). Personal Access Tokens
  (`dpk_` prefix) are accepted for non-browser clients and are stored only as
  hashes. Treat all of these as sensitive credentials; revoke any that leak.
- **Network exposure.** Deployik should sit behind TLS via the configured
  reverse proxy. Do not expose the API or the Docker socket directly to
  untrusted networks.

If you find a gap in this model, please report it through the channels above.
