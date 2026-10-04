import { buildReceipt } from "../about/about.remote";

export async function load() {
  return { receipt: await buildReceipt("atlas") };
}
