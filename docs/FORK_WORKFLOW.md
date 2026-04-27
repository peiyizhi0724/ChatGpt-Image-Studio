# Fork Workflow

This fork uses a split workflow so custom development and production deployment stay safe:

- `main`: integration branch for your custom features and upstream sync PRs
- `feature/*`: short-lived branches for individual changes
- `production`: deployment branch, only updated after validation

## Automatic upstream sync

The workflow at `.github/workflows/sync-upstream.yml` runs on schedule and by manual trigger.
It fetches `peiyizhi0724/ChatGpt-Image-Studio`, merges upstream `main` into a sync branch, and opens a PR back to this fork's `main`.

Recommended flow:

1. Review the sync PR.
2. Resolve any conflicts with your custom code.
3. Test the merged result on `main`.
4. Merge `main` into `production` only after validation.

## Automatic production deploy

The workflow at `.github/workflows/deploy-production.yml` deploys whenever `production` is updated.

The workflow at `.github/workflows/prepare-production-pr.yml` automatically opens or updates a PR from `main` to `production` whenever `main` changes.
That means your release flow becomes:

1. Review and merge upstream sync PRs into `main`.
2. Build or customize features on `main`.
3. Review the auto-generated `release: promote main to production` PR.
4. Merge that PR when you want to deploy.

Required repository secrets:

- `DEPLOY_HOST`: server IP or domain
- `DEPLOY_USER`: SSH user
- `DEPLOY_PORT`: SSH port
- `DEPLOY_SSH_PRIVATE_KEY`: private key used by GitHub Actions
- `DEPLOY_APP_DIR`: app path on the server, for example `/opt/projects/ChatGpt-Image-Studio`
- `HEALTHCHECK_URL`: optional healthcheck URL, for example `http://127.0.0.1:7000/health`

The server-side deploy script is `scripts/deploy-production.sh`.
It pulls the latest `production` branch, rebuilds the containers with `docker compose up -d --build`, and waits for the healthcheck to return success.

## One-time server setup

Run these commands on the server so the deployed code tracks your fork instead of the original repository:

```bash
cd /opt/projects/ChatGpt-Image-Studio
git remote set-url origin https://github.com/YuMu-i/ChatGpt-Image-Studio.git
git fetch origin
git checkout -B main origin/main
git checkout -B production origin/main
```

After that:

- keep building features on `main` or `feature/*`
- merge validated changes into `production`
- let GitHub Actions deploy `production` automatically
