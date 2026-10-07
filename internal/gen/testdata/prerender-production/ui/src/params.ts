import { defineParams } from '@sveltejs/kit/params';
export const params = defineParams({Order: (value: string) => /^\d+$/.test(value) && +value <= 1000000 ? +value : undefined});
