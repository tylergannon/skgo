import { defineParams } from "@sveltejs/kit/params";

export const params = defineParams({
  Order: (value: string) => {
    if (!/^[+-]?\d+$/.test(value)) return;
    const parsed = Number(value);
    if (parsed < 0 || parsed > 1000000) return;
    return parsed;
  },
});
