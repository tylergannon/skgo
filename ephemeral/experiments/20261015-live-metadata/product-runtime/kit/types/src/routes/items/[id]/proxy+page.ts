// @ts-nocheck
import type { PageLoad } from "./$types";
import { getItem } from "./item.remote";

// The ordinary route returns only the resolved value. The `retain` query is
// an explicit fixture for the separate active-reference single-flight check.
export const load = async ({ params, url }: Parameters<PageLoad>[0]) => {
  const item = getItem(params.id);
  const loadedItem = await item;
  return url.searchParams.has("retain") ? { loadedItem, item } : { loadedItem };
};
