# Contributing to Wobsongo

Thank you for your interest in contributing to Wobsongo Verify! We are an open-source project dedicated to building standard infrastructure for automated claim verification.

Whether you are fixing a bug, adding a new Vector DB connector, or improving our documentation, we welcome your help.

## Quick Start

### 1. Fork and Clone

Fork this repository to your own GitHub account and clone it locally:

```bash
git clone [https://github.com/ImpactScope-organization/wobsongo.git](https://github.com/ImpactScope-organization/wobsongo)
cd wobsongo
```

## 2. Set Up Environment

We use devbox for reproducible development environments. Initialize the environment and install all dependencies (including Go tooling) via our Makefile:

```bash
# Start the devbox environment
devbox shell

# Install Go modules and required tools
make setup

# Start local database dependencies via Docker
make dbup
```

## 3. Quality Assurance

All PRs must include tests and maintain **at least 80% line coverage**.

```bash
# Run only unit tests (skips integration tests)
make test-unit

# Run integration tests (requires local test DB)
make test-integration

# Run all tests with coverage report
make test
```

**Test conventions:**

- File names: *_test.go — place test files alongside the source module.
- Mocking: Use interface-based mocking.
- Test data: Use isolated fixtures in tests/data/ or generated mock data; never use real production data.

**Linting & Formatting:** Run before opening a PR — CI will reject on failures:

```bash
# Formats code and runs golangci-lint
make check
```

# Data Privacy & Safety Policy

**CRITICAL**: Do not commit any real user data, private health records, or proprietary knowledge base dumps to this repository.

- **Use Dummy Data**: When writing tests or examples, use the dummy data provided in tests/data/.
- **No API Keys**: Never hardcode OpenAI or Vector DB keys. Use environment variables (e.g., .env).

# Pull Request Process

- Clone/fork the repo. We recommend forking to your own GitHub account and cloning locally.
- Take on an issue or start a new feature. If you want to work on an existing issue, comment on it to let others know you're working on it.
- Create a new branch for your feature: `git checkout -b feature/my-new-feature`
- Commit your changes using clear messages: `git commit -m "feat(vector): add Qdrant support to Retriever"`
- Push to your branch: `git push origin feature/my-new-feature`
- Open a Pull Request against the main branch.
- Wait for a maintainer to review. We aim to review all PRs within 72 hours.
- Make sure that all checks pass (tests, linting, coverage) in GitHub Actions.
- Once you get an approval, one of the project maintainers will merge your PR.

# Reporting Bugs

If you find a bug, please open an issue on GitHub. Include:

- A clear title and description.
- Steps to reproduce the bug. A clone or a fork of the repo with reproduction steps is ideal.
- The version of Go, Devbox, and dependencies you are using. (although we always recommend using the versions in devbox.lock).

# Security Vulnerabilities

If you discover a security vulnerability, please do NOT open a public issue. Email [tahta@impactscope.com](mailto:tahta@impactscope.com) instead. We will work with you to patch it before disclosing it publicly.

By contributing to this project, you agree to abide by our Code of Conduct.
