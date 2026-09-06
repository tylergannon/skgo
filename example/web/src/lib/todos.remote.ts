import { query, command } from '$app/server';

export type Todo = { id: string; text: string };

// Every body throws. skgo answers these endpoints from Go, so anything that
// renders in the browser is proof the Go handler — not this module — replied.
const unimplemented = (): never => {
	throw new Error('skgo: implemented in Go');
};

export const getTodos = query('unchecked', (): Todo[] => unimplemented());

export const getTodo = query('unchecked', (_id: string): Todo => unimplemented());

export const addTodo = command('unchecked', (_text: string): Todo => unimplemented());

export const watchCount = query.live('unchecked', (): number => unimplemented());
