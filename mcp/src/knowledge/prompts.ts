import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { listRecipes } from "./index.js";

export function registerKnowledgePrompts(server: McpServer): void {
  for (const recipe of listRecipes()) {
    server.registerPrompt(
      `deployik_recipe_${recipe.topic.replace(/-/g, "_")}`,
      {
        title: recipe.title,
        description: recipe.summary,
      },
      async () => ({
        messages: [
          {
            role: "user",
            content: {
              type: "text",
              text: recipe.body,
            },
          },
        ],
      }),
    );
  }
}
