import { RECIPE_FILES } from "./recipes.generated.js";

export type RecipeTopic =
  | "overview"
  | "create-project"
  | "dockerfile-app"
  | "custom-domain"
  | "env-vars"
  | "auto-deploy"
  | "password-protection"
  | "contact-form-email"
  | "attach-postgres"
  | "rollback";

export interface Recipe {
  topic: RecipeTopic;
  title: string;
  summary: string;
  body: string;
}

const TOPIC_TITLES: Record<RecipeTopic, string> = {
  overview: "Deployik overview",
  "create-project": "Connect a GitHub repo and deploy it",
  "dockerfile-app": "Deploy a Dockerfile, Go server, API, or SQLite-backed app",
  "custom-domain": "Set up a custom domain with SSL",
  "env-vars": "Add environment variables and secrets",
  "auto-deploy": "Configure auto-deploy from GitHub",
  "password-protection": "Password-protect a preview or production site",
  "contact-form-email": "Wire up a contact form with email + reCAPTCHA",
  "attach-postgres": "Attach and manage a Postgres sidecar database",
  rollback: "Roll back a deployment",
};

const TOPIC_SUMMARIES: Record<RecipeTopic, string> = {
  overview: "What Deployik is and the dashboard's anatomy.",
  "create-project": "From 'I have a GitHub repo' to 'it's deployed' in seven clicks.",
  "dockerfile-app": "How to deploy Dockerfile/Go/custom long-running apps: use framework static, root_directory, port, and optional persistent volume.",
  "custom-domain": "DNS verification, SSL provisioning, and primary-domain selection.",
  "env-vars": "Shared, preview, and production scopes — when to use which.",
  "auto-deploy": "GitHub webhooks, preview/production branch matching, opt-in production fan-out.",
  "password-protection": "Generate a per-environment password and share the URL.",
  "contact-form-email": "Webglobe SMTP + reCAPTCHA v3 + the AI-install prompt for Next.js routes.",
  "attach-postgres": "Postgres attach/restart/credentials/reset workflow and safety gates.",
  rollback: "Promote a previous successful deployment back to live.",
};

const cached: Recipe[] | null = null;
let recipesCache: Recipe[] | null = cached;

function buildRecipes(): Recipe[] {
  if (recipesCache) return recipesCache;
  const fileContent: Record<string, string> = {};
  for (const f of RECIPE_FILES) {
    fileContent[f.file] = f.content;
  }

  const clickPaths = fileContent["click-paths.md"] ?? "";
  const apiActions = fileContent["api-actions.md"] ?? "";
  const skill = fileContent["SKILL.md"] ?? "";

  const clickSections = splitSections(clickPaths);
  const apiSections = splitSections(apiActions);

  const recipes: Recipe[] = [
    {
      topic: "overview",
      title: TOPIC_TITLES.overview,
      summary: TOPIC_SUMMARIES.overview,
      body: skill,
    },
  ];

  const topicHeaders: Array<{ topic: RecipeTopic; header: string }> = [
    { topic: "create-project", header: "create-project" },
    { topic: "dockerfile-app", header: "dockerfile-app" },
    { topic: "custom-domain", header: "custom-domain" },
    { topic: "env-vars", header: "env-vars" },
    { topic: "auto-deploy", header: "auto-deploy" },
    { topic: "password-protection", header: "password-protection" },
    { topic: "contact-form-email", header: "contact-form-email" },
    { topic: "attach-postgres", header: "attach-postgres" },
    { topic: "rollback", header: "rollback" },
  ];

  for (const { topic, header } of topicHeaders) {
    const click = clickSections[header];
    const api = apiSections[header];
    const parts: string[] = [];
    if (click) parts.push(`# ${TOPIC_TITLES[topic]}\n\n## Where to click\n\n${click.trim()}`);
    if (api) parts.push(`## API actions\n\n${api.trim()}`);
    if (parts.length === 0) continue;
    recipes.push({
      topic,
      title: TOPIC_TITLES[topic],
      summary: TOPIC_SUMMARIES[topic],
      body: parts.join("\n\n"),
    });
  }

  recipesCache = recipes;
  return recipes;
}

function splitSections(markdown: string): Record<string, string> {
  // Splits a markdown doc by `## <slug>` headers; section body is everything until the next `## ` or EOF.
  const sections: Record<string, string> = {};
  if (!markdown) return sections;
  const lines = markdown.split(/\r?\n/);
  let currentSlug: string | null = null;
  let buf: string[] = [];
  const flush = () => {
    if (currentSlug !== null) {
      sections[currentSlug] = buf.join("\n");
    }
  };
  for (const line of lines) {
    const m = /^##\s+(\S+)\s*$/.exec(line);
    if (m) {
      flush();
      currentSlug = m[1]!;
      buf = [];
      continue;
    }
    if (currentSlug !== null) buf.push(line);
  }
  flush();
  return sections;
}

export function listRecipes(): Recipe[] {
  return buildRecipes();
}

export function getRecipe(topic: RecipeTopic): Recipe | undefined {
  return buildRecipes().find((r) => r.topic === topic);
}

export const RECIPE_TOPICS: RecipeTopic[] = [
  "overview",
  "create-project",
  "dockerfile-app",
  "custom-domain",
  "env-vars",
  "auto-deploy",
  "password-protection",
  "contact-form-email",
  "attach-postgres",
  "rollback",
];
