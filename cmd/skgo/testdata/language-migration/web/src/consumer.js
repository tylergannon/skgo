import { receipt } from '#lib/data/data.remote.js';

/** @returns {Promise<import('./lib/data/types.js').Receipt>} */
export async function consumer() {
  const value = await receipt('checker');
  return { message: value.message, count: value.count };
}
