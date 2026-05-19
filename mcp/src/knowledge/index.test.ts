import { describe, expect, test } from "bun:test";

import { getRecipe, listRecipes, RECIPE_TOPICS } from "./index";

describe("Deployik recipes", () => {
  test("exposes Dockerfile and Postgres topics", () => {
    expect(RECIPE_TOPICS).toContain("dockerfile-app");
    expect(RECIPE_TOPICS).toContain("attach-postgres");
    expect(listRecipes().map((recipe) => recipe.topic)).toContain("dockerfile-app");
    expect(listRecipes().map((recipe) => recipe.topic)).toContain("attach-postgres");
  });

  test("documents Dockerfile deployment without a fake docker framework", () => {
    const recipe = getRecipe("dockerfile-app");

    expect(recipe?.body).toContain('framework: "static"');
    expect(recipe?.body).toContain("root_directory");
    expect(recipe?.body).toContain("Dockerfile");
    expect(recipe?.body).toContain("no special");
    expect(recipe?.body).toContain('framework: "docker"');
  });
});
