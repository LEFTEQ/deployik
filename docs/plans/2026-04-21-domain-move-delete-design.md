# Domain management: move, delete, re-verify, set primary

## Summary

Today `ProjectSettingsDomains.tsx` lets you add and verify domains but has no UI to **delete** a custom domain (even though `DELETE /api/projects/{id}/domains/{did}` exists) and there is **no way at all** — API or UI — to change a domain's environment or mark it as the preferred one for an environment. Users hit a dead end the moment they misclassify a domain or want to clean up.

This spec adds a kebab menu on every non-auto domain row that exposes **Move to {other env}**, **Re-verify**, **Set primary**, and **Delete**. The auto-generated `{project}.preview.example.com` stays immortal and immovable. Move re-provisions nginx + SSL because the variant plan can change across environments. Set-primary is enforced by a new DB column (one primary per project/environment).

## Decisions

| Question | Decision | Rationale |
|---|---|---|
| Action UX | **Kebab menu (three-dot)** on each domain row | Scales to 4 actions without cluttering the row; consistent pattern for future additions (regenerate cert, transfer, etc.). |
| Move backend | **`PATCH /api/projects/{id}/domains/{did}` + re-provision** | Preserves domain ID and audit trail; proactively syncs nginx/SSL so the system is never inconsistent. |
| Auto-domain rules | **Immortal + immovable** (no kebab menu) | `*.preview.example.com` is lovinka-managed infra; promoting it to production or deleting it makes no sense and would strand the project without a default URL. |
| Delete confirmation | **shadcn `AlertDialog`** naming the domain | Standard destructive-action pattern; prevents accidental click from the kebab menu. |
| Scope | Delete + Move + Re-verify + Set primary | Re-verify is free (endpoint exists); Set primary upgrades the implicit is_auto heuristic in `getPrimaryEnvironmentUrl` to explicit user intent. |

## Chosen approach

### Backend

1. **New migration `014_domain_primary.sql`**
   ```sql
   ALTER TABLE domains ADD COLUMN is_primary INTEGER NOT NULL DEFAULT 0;
   -- Backfill: one primary per (project, env). Preview → prefer auto, production → prefer first custom.
   UPDATE domains SET is_primary = 1 WHERE id IN (
     SELECT id FROM domains d1
     WHERE NOT EXISTS (
       SELECT 1 FROM domains d2
       WHERE d2.project_id = d1.project_id
         AND d2.environment = d1.environment
         AND (
           (d2.environment = 'preview' AND d2.is_auto > d1.is_auto) OR
           (d2.environment = 'production' AND d2.is_auto < d1.is_auto) OR
           (d2.is_auto = d1.is_auto AND d2.id < d1.id)
         )
     )
   );
   CREATE UNIQUE INDEX idx_domains_primary_per_env
     ON domains(project_id, environment) WHERE is_primary = 1;
   ```

2. **New endpoint: `PATCH /api/projects/{id}/domains/{did}`**
   Body: `{ environment?: "preview" | "production", is_primary?: true }`.
   - Rejects `is_auto=1` rows with 403.
   - **Environment change flow** (`DomainHandler.Update`):
     1. Load domain + project (authz).
     2. Validate new env != old env, refuse if another domain already claims the canonical hostname in the new env (unique constraint check).
     3. Compute old plan + new plan via `ResolveVariantPlan(domain, oldEnv)` vs `(domain, newEnv)`. If `AllDomains()` differs (e.g. apex moved from preview→production picks up a www variant), mark `dns_verified=false`, `ssl_status='pending'` so the frontend auto-verify kicks off a fresh DNS+SSL run.
     4. `Manager.RemoveDomain(oldDomain)` + `ReloadProxy()` for the old vhost.
     5. `db.UpdateDomainEnvironment(id, newEnv)`.
     6. If variant plan is unchanged: re-run `Manager.ProvisionDomain(...)` synchronously against the new environment's container (swaps the upstream in place, keeps SSL).
     7. If variant plan changed: skip re-provision — status is already 'pending' and the frontend auto-verify will call `POST /verify` which handles DNS + SSL end-to-end.
     8. Audit event `domain.move` with `{from, to, domain}`.
   - **Primary flag flow**: one transaction, `UPDATE domains SET is_primary=0 WHERE project_id=? AND environment=?` then `UPDATE domains SET is_primary=1 WHERE id=?`. Audit event `domain.set_primary`.

3. **Concurrency guard**: reuse `DomainHandler.verifying sync.Map` — if a verification is in progress for the project, Move returns 409. In-flight deploys that hold a domain are *not* blocked; the deploy targets by environment, not domain ID, and the subsequent re-provision updates nginx atomically.

4. **Helpers to add/update**
   - `db.UpdateDomainEnvironment(id string, env string) error`
   - `db.SetDomainPrimary(projectID, environment, domainID string) error` (transactional)
   - `db.ResetDomainVerification(id string) error` — sets `dns_verified=0`, `ssl_status='pending'`.
   - `Manager.RemoveDomain(oldDomainName)` already exists; keep it.

5. **Frontend primary preference**: `getPrimaryEnvironmentUrl` in `web/src/lib/deployment-helpers.ts` — add is_primary as the first sort key, falling back to today's `is_auto` heuristic so projects without explicit primaries continue to work.

### Frontend

1. **Kebab menu on custom rows** (`ProjectSettingsDomains.tsx`)
   - Use `DropdownMenu` from shadcn.
   - Items:
     - `Move to Production` / `Move to Preview` (label flips based on current env)
     - `Re-verify` (enabled even when ready; disabled while a verification is in flight)
     - `Set as primary` (hidden/disabled if row is already primary for its env)
     - Separator
     - `Delete` (red text)
   - Hide the menu trigger entirely for `is_auto=true` rows.

2. **Mutations** (added to existing page):
   ```ts
   const moveMutation = useMutation({
     mutationFn: ({ domainId, environment }) =>
       api.updateDomain(id, domainId, { environment }),
     onSuccess: () => queryClient.invalidateQueries(queryKeys.domains(id)),
   });
   const setPrimaryMutation = useMutation({
     mutationFn: (domainId) =>
       api.updateDomain(id, domainId, { is_primary: true }),
     onSuccess: () => queryClient.invalidateQueries(queryKeys.domains(id)),
   });
   const deleteMutation = useMutation({
     mutationFn: (domainId) => api.deleteDomain(id, domainId),
     onSuccess: () => queryClient.invalidateQueries(queryKeys.domains(id)),
   });
   ```

3. **Delete dialog** — `AlertDialog` wrapping the Delete menu item with the domain name in the title and a clear warning about nginx/SSL teardown.

4. **Primary indicator in row** — add a small `Primary` badge next to the Auto/Custom badge when `domain.is_primary`.

5. **Auto-verify interaction** — the move flow resets verification state when the variant plan changes. The existing auto-verify effect (added yesterday) picks that up and fires `POST /verify` automatically, so the user sees the log panel open right after picking "Move to Production" — no extra click needed.

### API client + types

- `web/src/types/api.ts` — add `is_primary: boolean` to `Domain`.
- `web/src/lib/api.ts` — add `updateDomain(projectId, domainId, patch)` calling `PATCH`.

## Architecture

```
User clicks [⋮] → "Move to Production" on forge.example.org
        │
        ▼
api.updateDomain(id, did, { environment: "production" })
        │
        ▼
PATCH /api/projects/:id/domains/:did
        │
        ├── authz.LoadProject ─── reject if !owner
        ├── reject is_auto
        ├── check unique(domain) free in new env
        ├── plan_old = ResolveVariantPlan(domain, "preview")
        ├── plan_new = ResolveVariantPlan(domain, "production")
        ├── Manager.RemoveDomain(domain) + ReloadProxy()
        ├── db.UpdateDomainEnvironment(did, "production")
        │
        ├── if plan_old.AllDomains() != plan_new.AllDomains():
        │     db.ResetDomainVerification(did)       ◄─ dns_verified=0, ssl='pending'
        │     respond 200; frontend auto-verify
        │     effect calls POST /verify which
        │     provisions nginx+SSL for the new
        │     variant plan.
        │
        └── else:
              Manager.ProvisionDomain(new env,
                container=deployik-{project}-production,
                keep existing SSL cert)
              respond 200 (SSL stays 'active')
```

## Open questions (deferred)

- **Does moving a domain mid-deploy break anything?** In practice no — deploys resolve domains by environment at dispatch time. But if a deploy is writing nginx for `preview` while we're removing a `preview` domain, the reload order matters. Mitigation: the pipeline's `ProvisionDomain` and this handler both call `ReloadProxy()` under the same in-memory `Manager`; worst case is an extra reload. Leaving unresolved for now.
- **Bulk move / bulk delete** — not in scope. If users start adding many domains the kebab menu is still the right primitive; bulk ops can layer on later.
- **Audit log UI** — `domain.move`, `domain.set_primary`, `domain.delete` are all recorded server-side but there's no in-app audit viewer. That's a separate feature.
- **Domain rename** — explicitly out of scope (user confirmed). Delete + re-add is the workflow.
