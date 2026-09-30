import type { PageLoad } from "./$types";

type Todo = { id: string; text: string; private: boolean };

export const load: PageLoad = async ({ fetch, url }) => {
  const response = await fetch(
    `/api/todos?via=universal&refresh=${url.searchParams.get("refresh") ?? "0"}`,
    { headers: { "x-demo": "universal", accept: "application/json" } },
  );
  const publicResponse = await fetch("/api/todos", { credentials: "omit" });
  if (!response.ok || !publicResponse.ok) throw new Error("The todos API did not answer");
  return {
    todos: (await response.json()) as Todo[],
    publicTodos: (await publicResponse.json()) as Todo[],
    hook: response.headers.get("x-fetch-hook") ?? "browser",
  };
};
