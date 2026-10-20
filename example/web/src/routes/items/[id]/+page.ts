import type { PageLoad } from "./$types";
import { getItem } from "./item.remote";

// The ordinary route returns only the resolved value. The `retain` query is
// the explicit fixture for the active-reference checks: page data keeps the
// query reachable, so Kit's client keeps its cache entry for the component.
export const load: PageLoad = async ({ params, url }) => {
  const item = getItem(params.id);
  const loadedItem = await item;
  return url.searchParams.has("retain") ? { loadedItem, item } : { loadedItem };
};
