import type { PageLoad } from "./$types";

// A universal load fetches a whole rendered page of the app. During the
// document render that is a second render, nested inside this one.
export const load: PageLoad = async ({ fetch }) => {
  const response = await fetch("/request-fetch");
  if (!response.ok) throw new Error("The request-fetch page did not answer");
  const html = await response.text();
  const fact = /data-testid="request-fetch-fact">([^<]+)</.exec(html)?.[1] ?? "not found";
  const visitor =
    /data-testid="request-fetch-visitor">Fetched as ([^<]+)</.exec(html)?.[1] ?? "not found";
  return { fact, visitor, status: response.status };
};
