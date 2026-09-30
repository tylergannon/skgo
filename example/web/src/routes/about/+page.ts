// The root layout's Go load runs in Kit's build, and this page is the static
// file Kit writes from its result.
export const prerender = true;

import { buildReceipt } from "./about.remote";

export async function load({ data }: import("./$types").PageLoadEvent) {
  return { ...data, remoteReceipt: await buildReceipt("atlas") };
}
