# Release and Container Guidelines

> Project rules for version tags, container image publication, production update records, and rollback identity.

---

## Goals

- Normal commits and branch pushes run CI only; they must not publish production container images.
- Only an explicit, validated release tag may build and push a deployable image.
- Every deployable image must be traceable to one Git tag, one Git commit, the source tree, migration SQL, and a build record.
- Do not rely on `latest` as the only production version.
- Every deployable image must be rollback-capable by immutable digest.

---

## Release Tag Format

This repository uses the `cafecode-` release tag prefix for the production container line.

Allowed stable tags:

```text
cafecode-vMAJOR.MINOR.PATCH
```

Examples:

```text
cafecode-v1.0.0
cafecode-v1.2.3
cafecode-v2.0.0
```

Allowed prerelease tags:

```text
cafecode-vMAJOR.MINOR.PATCH-alpha.N
cafecode-vMAJOR.MINOR.PATCH-beta.N
cafecode-vMAJOR.MINOR.PATCH-rc.N
```

Examples:

```text
cafecode-v1.3.0-alpha.1
cafecode-v1.3.0-beta.2
cafecode-v1.3.0-rc.1
```

The enforced pattern is:

```text
^cafecode-v[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)\.[0-9]+)?$
```

Do not use ambiguous release tags:

```text
latest
prod
release
v1
v1.2
v1.2.3
20240704
```

Use annotated tags for human-created releases:

```bash
git tag -a cafecode-v1.2.3 -m "Release cafecode-v1.2.3"
git push origin cafecode-v1.2.3
```

Once a tag is published, do not delete and recreate it. If a release is bad, publish a new tag such as `cafecode-v1.2.4`.

---

## GitHub Actions Release Rules

Container image publication is handled by:

```text
.github/workflows/custom-prod-image.yml
```

It must be triggered only by:

- push of a valid `cafecode-v*` tag, or
- explicit `workflow_dispatch` with a valid release tag input.

Branch pushes such as `custom-prod` must not publish production images. They should run test/security workflows only.

The legacy GoReleaser workflow:

```text
.github/workflows/release.yml
```

is a manual emergency/legacy path and must be disabled by default. It must not be used unless repository variable `ENABLE_LEGACY_GORELEASER=true` is deliberately set for a one-off legacy release.

---

## Container Image Tagging Rules

Primary registry:

```text
ghcr.io/<owner>/<repo>
```

For stable Git tag `cafecode-v1.2.3`, publish:

```text
ghcr.io/<owner>/<repo>:cafecode-v1.2.3
ghcr.io/<owner>/<repo>:1.2.3
ghcr.io/<owner>/<repo>:1.2
ghcr.io/<owner>/<repo>:latest
ghcr.io/<owner>/<repo>:sha-<short-commit>
```

For prerelease Git tag `cafecode-v1.3.0-rc.1`, publish:

```text
ghcr.io/<owner>/<repo>:cafecode-v1.3.0-rc.1
ghcr.io/<owner>/<repo>:1.3.0-rc.1
ghcr.io/<owner>/<repo>:rc
ghcr.io/<owner>/<repo>:sha-<short-commit>
```

Prereleases may use the channel tags `alpha`, `beta`, or `rc`, but must not update:

```text
latest
MAJOR.MINOR
```

The `latest` tag is allowed only for stable releases and is a convenience pointer, not a rollback identity.

---

## Image Architecture Policy

Production release images are built for the production host architecture only:

```text
linux/amd64
```

Do not enable multi-architecture release builds by default. In particular, do not add `linux/arm64` to the production release workflow unless there is an explicit deployment requirement and the slower build time is accepted.

Reason: GitHub-hosted `ubuntu-latest` runners are amd64; adding `linux/arm64` makes Buildx use QEMU emulation and can turn a normal ~3 minute image publication into a much longer multi-arch build.

---

## Build Record Requirements

Every image publication must upload a release record artifact containing at least:

- Git tag
- Git commit SHA
- image name
- image tags
- image digest
- build time
- workflow run URL
- migration SQL file list hash
- migration SQL content list hash
- backfill SQL file list hash
- backfill SQL content list hash
- latest migration file in the image source tree

The reliable deploy identity is:

```text
Git tag + Git commit SHA + image digest + DB migration state
```

---

## Production Update Discipline

AI assistants and automation must not update production containers by default.

For code tasks, only edit code/configuration, run local or CI-oriented tests, commit, and push. Remote CI/CD may build images after release tags; the human operator decides when production is updated.

Do not run production lifecycle commands unless explicitly asked for an allowed rollback/restore action. Forbidden by default:

```bash
docker compose up
docker compose down
docker compose restart
docker compose pull
docker restart
docker stop
docker rm
systemctl restart
kubectl apply
kubectl rollout restart
./deploy.sh
./update.sh
```

Read-only inspection is allowed when needed for diagnosis or rollback preparation:

```bash
docker ps
docker inspect
```

When uncertain whether an action changes production state, ask before running it.

---

## Pre-Update Record Checklist

Before any production update, capture the current rollback baseline and append it to the local container history document:

```text
容器更新历史.md
```

Record:

- container name and container ID
- current image tag
- immutable image ID / digest
- full rollback image reference such as `ghcr.io/owner/repo@sha256:...`
- container status and health
- container created time
- image created time
- current Git branch and commit of the local deployment checkout
- current DB migration state from `schema_migrations`

Also snapshot already-applied migration SQL when a migration-bearing update is planned. The snapshot must include filenames, checksums, applied times, and the SQL content whose checksum matches the DB record.

---

## Production Deployment Rule

Prefer deploying by digest:

```text
ghcr.io/<owner>/<repo>@sha256:<digest>
```

If deployment tooling still uses a tag, record both:

```text
image tag: ghcr.io/<owner>/<repo>:cafecode-v1.2.3
digest: sha256:<digest>
git sha: <commit>
latest applied migration: <filename>
```

Do not rely on `latest` alone for production or rollback.

---

## Rollback Rule

Allowed exception: if the user explicitly asks to roll back or restore production to a specific previous version, image, digest, or known-good state, perform only the minimal rollback actions required and then verify health.

Do not upgrade, pull `latest`, deploy a new build, or perform unrelated changes during rollback.

If rollback may require database downgrade, destructive data changes, or reverse migrations, stop and explain the risk before taking action.

---

## Review Checklist

When reviewing release or container workflow changes:

- [ ] Branch pushes cannot publish production images.
- [ ] Release tags are validated with the strict SemVer-style pattern.
- [ ] Prereleases do not update `latest` or `MAJOR.MINOR`.
- [ ] Stable releases publish immutable version tags and may update `latest`.
- [ ] Release image platforms remain `linux/amd64` unless multi-arch is explicitly required.
- [ ] Build records include image digest and migration SQL hashes.
- [ ] No workflow can bypass the tag-driven release path unless it is explicitly disabled by default and guarded.
- [ ] Production update instructions use image digests for deployment and rollback identity.
- [ ] Migration state is recorded alongside image version before updates.
