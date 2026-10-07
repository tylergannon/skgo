import type { PageLoad } from "./$types";
import { getItem } from "./item.remote";

// The ordinary route returns only the resolved value. The `retain` query is
// an explicit fixture for the separate active-reference single-flight check.
export const load: PageLoad = async ({ params, url }) => {
  const item = getItem(params.id);
  const loadedItem = await item;
  return url.searchParams.has("retain") ? { loadedItem, item } : { loadedItem };
};
