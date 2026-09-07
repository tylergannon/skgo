// See https://svelte.dev/docs/kit/types#app.d.ts for information about these
// interfaces.
declare global {
	namespace App {
		/**
		 * Augments kit's own `{ status, message }` with the one extra field
		 * this app's `handleError` hook adds to every error it sees — the Go
		 * half is example.HandleError, in example/server.go. A field declared
		 * here is what makes `page.error.supportId` type-check in
		 * src/routes/+error.svelte; without it, the hook could still return
		 * the field at runtime, but nothing in the app could read it back.
		 */
		interface Error {
			supportId?: string;
		}
		// interface Locals {}
		// interface PageData {}
		// interface PageState {}
		// interface Platform {}
	}
}

export {};
