# Auto Deploy Production Opt-In Design

## Context

Deployik already supports auto-build through GitHub push webhooks. The current model has one enabled flag plus two branch settings:

- `production_branch`: pushes to this branch deploy production.
- `preview_branches`: pushes to matching non-production branches deploy preview.

That makes a push mutually exclusive: a branch deploys either production or preview. For simple websites, the desired workflow is different. The default branch should keep the preview deployment fresh automatically, and projects can explicitly opt in to also deploy production from each push to that same branch. In that mode, preview and production intentionally track the same commit, while still remaining separate Deployik environments, images, containers, domains, and environment-variable scopes.

## Goals

- Make preview auto-deploy the default behavior for branch pushes.
- Add an explicit opt-in for production auto-release on each push to the configured production branch.
- Allow one webhook event to enqueue both preview and production deployments from the same commit.
- Avoid creating Git tags or release versions on every commit.
- Add the new opt-in to the new project wizard.
- Migrate existing projects to the safer default: preview auto-build remains enabled, production auto-release is off until explicitly enabled.

## Non-Goals

- No artifact promotion or build-once/run-twice pipeline change.
- No Git tag creation for automatic production deploys.
- No generalized multi-rule automation engine.
- No changes to manual production deployment or manual tag creation behavior.

## Product Behavior

Auto-build should be described as preview-first:

- When auto-build is enabled, pushes to matching preview branches create preview deployments.
- When production auto-release is enabled, pushes to the production branch create an additional production deployment from the same commit.
- A default branch push can therefore create two deployments:
  - `preview` from `main@sha`
  - `production` from `main@sha`
- The two deployments build separately and produce separate images. This preserves current isolation and avoids build-time environment leakage between preview and production.
- Production auto-release is off by default everywhere.

## Settings Page

The existing Auto-Build section should keep the main enable switch and branch controls, but clarify the behavior:

- Section title can remain `Auto-Build on Push`.
- Main switch means `Auto-deploy preview on push`.
- `Production Branch` remains the branch used by production automation and defaults to the project branch.
- `Preview Branches` continues to accept `*` or comma-separated branch names.
- Add a secondary switch: `Auto-release production from production branch`.
- Helper text: production auto-release is for simple websites where preview and production are expected to track the same commit.

When the secondary switch is off, pushes to `main` create preview only. When it is on, pushes to `main` create preview and production.

## New Project Wizard

Add an Auto-Deploy section to the Configure Project step after the branch selection and before build settings:

- `Auto-deploy preview on push`: enabled by default.
- `Also deploy production on every push to this branch`: unchecked by default.
- The branch is the selected project branch, usually the repository default branch.

On project creation:

- Deployik should still best-effort provision the GitHub webhook and auto-build config.
- The created auto-build config should set preview auto-build enabled by default.
- The new production auto-release flag should reflect the wizard checkbox.
- Initial deployment should always enqueue preview when the pipeline is available.
- If production auto-release is checked, initial project creation should also enqueue a production deployment from the same branch.

If webhook provisioning fails due to GitHub scope or repository admin access, project creation should continue as it does today.

## Backend Design

Add a new boolean to `auto_build_configs`:

```sql
auto_production_enabled INTEGER NOT NULL DEFAULT 0
```

The API response and update request should include this field. Existing API consumers that do not send it get the default `false`.

Webhook handling changes:

1. Verify the GitHub signature and match the project config exactly as today.
2. If the branch matches `preview_branches`, enqueue a preview deployment.
3. If `auto_production_enabled` is true and the branch equals `production_branch`, enqueue a production deployment.
4. If neither condition matches, record the webhook event as ignored.

For the default branch with `preview_branches = "*"`, both conditions can be true. The handler should create two deployment records and dispatch both through the current pipeline.

Webhook event tracking currently has a unique `github_delivery_id`, which is too narrow when one delivery can produce multiple project/environment outcomes. Change the table to store one row per project/environment outcome:

- add an `environment` column with values `preview`, `production`, or `ignored`;
- replace the globally unique `github_delivery_id` constraint with uniqueness across `github_delivery_id`, `project_id`, and `environment`;
- link each processed row to its deployment through the existing `deployment_id` field.

This keeps webhook audit rows aligned with the deployment records they create.

## Data Migration

Create a forward migration that adds the new boolean with default `0`.

Existing projects:

- Keep `auto_build_configs.enabled` as-is.
- Set `auto_production_enabled = 0`.
- Keep branch values unchanged.

This intentionally changes enabled configs that previously auto-deployed production from `main` into preview-only automation until the owner opts back into production auto-release.

## UI/API Types

Update:

- Go `AutoBuildConfig`
- Auto-build request/response structs
- auto-build database queries
- frontend `AutoBuildConfig` type
- `api.updateAutoBuildConfig`
- `ProjectSettings` Auto-Build section
- `NewProject` create payload and corresponding backend create request

## Testing

Backend tests should cover:

- Existing configs migrate with production auto-release off.
- `main` push with auto-build enabled and production auto-release off enqueues preview only.
- `main` push with production auto-release on enqueues preview and production.
- Non-matching branch is ignored.
- Project creation defaults to preview auto-build enabled and production auto-release off.
- Project creation with production auto-release on enqueues initial preview and production deployments.

Frontend checks should cover:

- Settings page renders the new switch with correct default state.
- Saving settings persists the new flag.
- New project wizard sends the selected production auto-release flag.

## Rollout Notes

This is a database migration and behavior change. Back up production SQLite data before applying the migration. After deploy, existing projects should be reviewed in the UI if any are expected to keep automatic production deployments.
