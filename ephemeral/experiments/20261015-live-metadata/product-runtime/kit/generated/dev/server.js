
import error from './shared/error-template.js';

export const options = {
	app_template_contains_nonce: false,
	csp: {"mode":"auto","directives":{"script-src":["self"],"upgrade-insecure-requests":false,"block-all-mixed-content":false},"reportOnly":{"upgrade-insecure-requests":false,"block-all-mixed-content":false}},
	csrf_trusted_origins: ["https://trusted.test"],
	service_worker_options: undefined,
	templates: {
		app: ({ head, body, assets, nonce, env }) => "<!doctype html>\n<html lang=\"en\">\n  <head>\n    <meta charset=\"utf-8\" />\n    <meta name=\"viewport\" content=\"width=device-width, initial-scale=1\" />\n    " + head + "\n  </head>\n  <body>\n    <div style=\"display: contents\">" + body + "</div>\n  </body>\n</html>\n",
		error
	}
};

export async function get_hooks() {
	let handle;
	let handleFetch;
	let handleError;
	let init;
	({ handle, handleFetch, handleError, init } = await import("../../../../../../../example/web/src/hooks.server.ts"));

	let reroute;
	let transport;
	({ reroute, transport } = await import("../../../../../../../example/web/src/hooks.ts"));

	return {
		handle,
		handleFetch,
		handleError,
		init,
		reroute,
		transport
	};
}
