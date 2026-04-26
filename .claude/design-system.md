# Deployik Design System

## Stack

| Area | Choice |
|---|---|
| Framework | React 19 + Vite + TanStack Router |
| UI | shadcn/ui `new-york`, Radix primitives, Tailwind CSS 4 |
| Theme | Zinc dark admin UI with semantic CSS variables |
| Icons | `lucide-react` |
| Charts | Recharts via `components/ui/chart.tsx` |

## Tokens

| Category | Source | Notes |
|---|---|---|
| Color | `web/src/styles.css` | Use semantic tokens: `background`, `card`, `muted`, `primary`, `success`, `warning`, `destructive`, `border`, chart tokens. |
| Radius | `--radius: 0.5rem` | Cards and panels stay compact: `rounded-lg` is the normal container radius. |
| Typography | Tailwind theme | `Inter` for UI, `JetBrains Mono` for code, env keys, SHAs, and technical values. |
| Layout | Existing pages | Admin pages use constrained content (`max-w-[1400px]`), dense cards, compact buttons, and sidebar navigation. |

## Core Components

| Component | Path | Use |
|---|---|---|
| Sidebar shell | `web/src/components/layout/AppSidebar.tsx` | Workspace/project navigation, collapsible project sections. |
| Project shell | `web/src/components/layout/ProjectLayout.tsx` | Breadcrumb header, fast deploy actions, project content width. |
| Variable store | `web/src/components/projects/variable-store.tsx` | Canonical env/secret CRUD UX. |
| Code panel | `web/src/components/ui/code-panel.tsx` | Copyable snippets and AI prompts. |
| Analytics stat/chart | `web/src/components/analytics/*` | Dashboard KPI and chart primitives. |

## Patterns

| Pattern | Guidance |
|---|---|
| Project navigation | Primary project features are top-level or collapsible groups. Integrations are a default-open group with concrete children such as Analytics and Email. |
| Integration pages | Keep configuration, env/secret setup, verification, install prompt, and help/how-to in one project-scoped page. |
| Forms | Use existing shadcn form controls, compact labels, and `flex flex-col gap-*` stacks. |
| Secrets | Secret values are never echoed back. Show presence status and only sync a secret when the user provides a fresh value. |
| Help content | Keep operational help short, task-specific, and colocated with the integration workflow. |
