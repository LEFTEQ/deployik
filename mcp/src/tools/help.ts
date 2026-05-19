import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { registerTool, type ToolContext } from "./_helpers.js";
import { getRecipe, listRecipes, RECIPE_TOPICS, type RecipeTopic, type Recipe } from "../knowledge/index.js";

export function registerHelpTools(server: McpServer, ctx: ToolContext): void {
  registerTool(server, ctx, {
    name: "list_recipes",
    description:
      "List bundled Deployik how-to recipes available via `get_recipe`. Use these to self-onboard the AI without needing the user to paste docs.",
    annotations: { readOnlyHint: true },
    handler: async () => {
      const recipes = listRecipes();
      const text = recipes.map((r) => `  • ${r.topic.padEnd(22)}  ${r.title}\n      ${r.summary}`).join("\n");
      return {
        text,
        data: recipes.map((r) => ({ topic: r.topic, title: r.title, summary: r.summary })),
      };
    },
  });

  registerTool(server, ctx, {
    name: "get_recipe",
    description: "Get the full Deployik recipe for a topic. Topics come from the bundled `deployik-howto` skill.",
    inputSchema: {
      topic: z.enum(RECIPE_TOPICS as [RecipeTopic, ...RecipeTopic[]]),
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const recipe = getRecipe(args.topic);
      if (!recipe) {
        return { text: `Unknown topic: ${args.topic}. Call list_recipes for the catalog.`, isError: true };
      }
      return { text: recipe.body, data: { topic: recipe.topic, title: recipe.title } };
    },
  });

  registerTool(server, ctx, {
    name: "find_help",
    description:
      "Answer 'where do I set this?' / 'how do I X?' style questions by ranking the bundled Deployik recipes against your question. Returns the best matching recipe in full plus a short list of runners-up. Use this when you don't know which exact topic to ask for — just describe the goal in plain English.",
    inputSchema: {
      question: z.string().describe("Plain-English question or goal, e.g. 'where do I set Stripe API keys for the live site?'"),
      max_results: z.number().int().positive().max(8).default(3),
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const ranked = rankRecipes(args.question);
      if (ranked.length === 0) {
        return {
          text: `No matching recipe found for '${args.question}'. Call list_recipes to see the full catalog.`,
          data: { matches: [] },
        };
      }
      const top = ranked[0]!;
      const runnersUp = ranked.slice(1, args.max_results);
      const summary = [
        `Best match: ${top.recipe.title} (topic: ${top.recipe.topic}, score: ${top.score})`,
        `Why: ${top.matchedTerms.length > 0 ? "matched terms — " + top.matchedTerms.join(", ") : "general overlap"}`,
        ``,
        `── Recipe body ──`,
        top.recipe.body,
      ];
      if (runnersUp.length > 0) {
        summary.push(``, `── Other relevant topics ──`);
        for (const r of runnersUp) {
          summary.push(`  • ${r.recipe.topic.padEnd(22)} ${r.recipe.title}  (score: ${r.score})`);
        }
        summary.push(``, `Call get_recipe({ topic: "<name>" }) to fetch any of those in full.`);
      }
      return {
        text: summary.join("\n"),
        data: {
          best: { topic: top.recipe.topic, title: top.recipe.title, score: top.score, matched: top.matchedTerms },
          runners_up: runnersUp.map((r) => ({ topic: r.recipe.topic, title: r.recipe.title, score: r.score })),
        },
      };
    },
  });
}

interface RankedRecipe {
  recipe: Recipe;
  score: number;
  matchedTerms: string[];
}

const STOPWORDS = new Set([
  "the","a","an","is","are","do","does","did","i","me","my","we","our","you","your",
  "to","of","in","on","at","for","with","this","that","these","those","how","where",
  "what","when","why","which","can","could","should","would","please","just","like",
  "set","setting","add","change","update","make","need","want","get","go","be","new",
  "deployik","app","site","project","value","using","via","up","down",
]);

const SYNONYMS: Record<string, string> = {
  // env vars
  env: "envvars", envs: "envvars", environment: "envvars", variable: "envvars", variables: "envvars",
  config: "envvars", configuration: "envvars", "next-public": "envvars",
  // secrets
  secret: "envvars", secrets: "envvars", key: "envvars", apikey: "envvars",
  stripe: "envvars", token: "envvars", password: "envvars",
  // domain
  domain: "domain", domains: "domain", url: "domain", ssl: "domain", dns: "domain",
  custom: "domain", subdomain: "domain", https: "domain", cert: "domain", certificate: "domain",
  // auto-deploy
  autodeploy: "autodeploy", webhook: "autodeploy", auto: "autodeploy", push: "autodeploy",
  github: "autodeploy", branch: "autodeploy", trigger: "autodeploy",
  // password protection
  protect: "protection", protection: "protection", protected: "protection", lock: "protection",
  preview: "preview", private: "protection",
  // email / contact form
  email: "email", contact: "email", smtp: "email", recaptcha: "email", form: "email",
  webglobe: "email", mail: "email", "form-email": "email",
  // dockerfile / custom servers
  docker: "dockerfile", dockerfile: "dockerfile", container: "dockerfile", image: "dockerfile",
  go: "dockerfile", golang: "dockerfile", api: "dockerfile", server: "dockerfile",
  sqlite: "dockerfile", volume: "dockerfile", persistent: "dockerfile",
  // postgres sidecar
  postgres: "postgres", postgresql: "postgres", database: "postgres", db: "postgres", sidecar: "postgres",
  // rollback
  rollback: "rollback", revert: "rollback", undo: "rollback", restore: "rollback", previous: "rollback",
  // create / connect project
  connect: "create", create: "create", new: "create", setup: "create", import: "create",
  repo: "create", repository: "create", deploy: "create", first: "create",
};

function rankRecipes(question: string): RankedRecipe[] {
  const tokens = tokenize(question);
  if (tokens.length === 0) return [];

  // Expand tokens through the synonym map (a token contributes both itself and any
  // canonical bucket it belongs to).
  const expanded = new Set<string>();
  for (const t of tokens) {
    expanded.add(t);
    if (SYNONYMS[t]) expanded.add(SYNONYMS[t]);
  }

  const ranked: RankedRecipe[] = [];
  for (const recipe of listRecipes()) {
    const haystack = (recipe.topic + " " + recipe.title + " " + recipe.summary + " " + recipe.body).toLowerCase();
    let score = 0;
    const matched: string[] = [];
    for (const term of expanded) {
      if (!haystack.includes(term)) continue;
      // Weight: matches in topic/title are worth more than body matches.
      const inTopic = recipe.topic.toLowerCase().includes(term);
      const inTitle = recipe.title.toLowerCase().includes(term);
      const inSummary = recipe.summary.toLowerCase().includes(term);
      const weight = inTopic ? 5 : inTitle ? 3 : inSummary ? 2 : 1;
      score += weight;
      matched.push(term);
    }
    if (score > 0) ranked.push({ recipe, score, matchedTerms: matched });
  }
  ranked.sort((a, b) => b.score - a.score);
  return ranked;
}

function tokenize(input: string): string[] {
  const out: string[] = [];
  for (const raw of input.toLowerCase().split(/[^a-z0-9]+/)) {
    if (!raw) continue;
    if (raw.length < 2) continue;
    if (STOPWORDS.has(raw)) continue;
    out.push(raw);
  }
  return out;
}
