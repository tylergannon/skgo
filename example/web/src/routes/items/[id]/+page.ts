import type { PageLoad } from "./$types";
import { getItem } from "./item.remote";

// Kit's query cache entry lives only as long as a proxy for it does (a
// FinalizationRegistry evicts it once every proxy is collected), and a load's
// proxy is otherwise garbage the moment it is awaited. Handing it to the page
// keeps the entry alive until the component asks for the same query, so the
// navigation makes one request however long the response takes.
export const load: PageLoad = async ({ params }) => {
  const item = getItem(params.id);
  return { loadedItem: await item, item };
};
