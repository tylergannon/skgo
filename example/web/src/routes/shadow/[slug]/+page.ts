export const prerender = true;

// Kit asks for these parameters during route analysis, before prerendering.
export function entries() {
  return [{ slug: "fixed" }];
}
