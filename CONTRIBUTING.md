# Contributing to Wobsongo

Thank you for your interest in contributing to Wobsongo! We are an open-source project building infrastructure for automated claim verification. Whether you are fixing a bug, improving documentation, or adding a new feature, we welcome your help.

> For the full contribution guide including testing conventions and data privacy policy, see the [documentation site](https://impactscope-organization.github.io/wobsongo/governance/CONTRIBUTING/).

## Quick Start

### 1. Fork and Clone

```bash
git clone https://github.com/ImpactScope-organization/wobsongo.git
cd wobsongo
```

### 2. Set Up Environment

We use [Devbox](https://www.jetify.com/docs/devbox/quickstart/) for reproducible development environments:

```bash
devbox shell
make dbup
make migrateup
```

### 3. Make Your Change

Create a branch and commit using [Conventional Commits](https://www.conventionalcommits.org/):

```bash
git checkout -b feature/my-feature
git commit -m "feat(api): add document ingestion endpoint"
```

### 4. Quality Checks

All PRs must pass the following before review:

```bash
make check        # Lint and build
make test         # Full test suite (requires make dbtestup first)
```

Maintain at least **80% line coverage** for changed packages.

### 5. Open a Pull Request

Push your branch and open a PR against `main`. A maintainer will review within 72 hours.

## Reporting Bugs

Open an issue on [GitHub](https://github.com/ImpactScope-organization/wobsongo/issues) with:

- A clear title and description
- Steps to reproduce
- Your Go and Devbox versions

## Security Vulnerabilities

Do **not** open a public issue. See [SECURITY.md](SECURITY.md) for the responsible disclosure process.

## Code of Conduct

By contributing, you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md).
