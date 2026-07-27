# Quick Reference

Quick access to common commands, patterns, and workflows for Wobsongo development.

## Essential Commands

### Daily Workflow

```bash
# Start development environment
devbox shell
make dev
```

### Code Quality

```bash
make fmt          # Format code
make check        # Lint and build
make test         # Run tests

```

## Git Workflow Cheat Sheet

```bash
# Start new feature
git checkout main
git pull origin main
git checkout -b feature/document-upload

# Make changes and commit using Conventional Commits
git add .
# Use Conventional Commits format
git commit -m "feat(api): add document upload endpoint"
git commit -m "fix(auth): resolve JWT expiration bug"

# For breaking changes
git commit -m "feat(api)!: redesign claim response structure"

# Push to remote
git push -u origin feature/document-upload

```

## Makefile Commands Cheat Sheet

```bash
# Code Quality
make fmt           # Format code
make check         # Lint, swagger, build
make gen           # Generate code, swagger docs

# Development
make dev           # Start with hot-reload

# Database
make dbup          # Start database
make dbstop        # Stop database
make dbdown        # Remove database
make dball         # Full reset
make migrateup     # Run migrations
make reset         # Reset with data

# Testing
make test          # Run all tests
make dbtestup      # Start test database
make dbtestdown    # Stop test database
```