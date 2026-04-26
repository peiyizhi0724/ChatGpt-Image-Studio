# Upstream Sync Guide

This project started from the upstream repository:

- `https://github.com/peiyizhi0724/ChatGpt-Image-Studio`

If you want to keep your customized version and still sync with upstream later, use the workflow below.

## Recommended Setup

### 1. Keep your own remote

Use your own repository as `origin`.

Example:

```bash
git remote rename origin upstream
git remote add origin https://github.com/<your-name>/<your-repo>.git
```

After that:

- `origin` = your repository
- `upstream` = original open-source repository

## Daily Workflow

### Push your own work

```bash
git push origin main
```

### Fetch upstream changes

```bash
git fetch upstream
```

### Rebase your branch on top of upstream

```bash
git checkout main
git rebase upstream/main
```

If you prefer merge instead of rebase:

```bash
git checkout main
git merge upstream/main
```

Then push your updated branch:

```bash
git push origin main
```

If you used rebase and your remote already has the old history:

```bash
git push --force-with-lease origin main
```

## If You Want to Contribute Back to the Original Project

The usual approach is:

1. fork the upstream repository on GitHub
2. push your branch to your own fork
3. open a Pull Request from your fork to upstream

Recommended command flow:

```bash
git checkout -b feature/<short-name>
git push origin feature/<short-name>
```

Then create a Pull Request on GitHub.

## What to Contribute Back

Good upstream candidates from this customized version:

- stable image concurrency control
- account lease protection
- quota refresh TTL
- startup diagnostics
- runtime request log improvements

Things that are often better kept local:

- personal proxy defaults
- local startup scripts
- desktop shortcut helpers
- your own operational preferences

## Before Opening a Pull Request

Make sure you:

1. remove local-only files
2. keep `.gitignore` clean
3. avoid committing auth files or config secrets
4. write a clear change summary
5. explain why the upstream project benefits from the change

## Useful Commands

Show remotes:

```bash
git remote -v
```

Add upstream later:

```bash
git remote add upstream https://github.com/peiyizhi0724/ChatGpt-Image-Studio.git
```

Fetch all remotes:

```bash
git fetch --all
```

See what changed compared with upstream:

```bash
git log --oneline upstream/main..main
```

See file diff compared with upstream:

```bash
git diff upstream/main...main
```
