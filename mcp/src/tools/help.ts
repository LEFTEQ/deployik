import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { registerTool, type ToolContext } from "./_helpers.js";
import { getRecipe, listRecipes, RECIPE_TOPICS, type RecipeTopic } from "../knowledge/index.js";

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
}
