// The same load with `csr = false`: no script reaches the browser, so nothing
// is serialized for it and Kit does not guard what a load may read.
export const csr = false;
export { load } from "../+page";
