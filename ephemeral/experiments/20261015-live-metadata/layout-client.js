import { createHotContext as __vite__createHotContext } from "/@vite/client";import.meta.hot = __vite__createHotContext("/src/routes/+layout.svelte");import "/@fs/private/var/folders/lt/09rsy64x65s_0fp2b8zq3n7m0000gn/T/skgo-dev-2412119449/vite/vite-cache/deps/svelte_internal_disclose-version.js?v=bd89f3c9";
import "/@fs/private/var/folders/lt/09rsy64x65s_0fp2b8zq3n7m0000gn/T/skgo-dev-2412119449/vite/vite-cache/deps/svelte_internal_flags_async.js?v=bd89f3c9";

_layout[$.FILENAME] = 'src/routes/+layout.svelte';

import * as $ from "/@fs/private/var/folders/lt/09rsy64x65s_0fp2b8zq3n7m0000gn/T/skgo-dev-2412119449/vite/vite-cache/deps/svelte_internal_client.js?v=bd89f3c9";
import { building } from "/node_modules/.pnpm/@sveltejs+kit@3.0.0_patch_hash=0d35d370d3e5fb6e0c801cd1079013b3d487d6e301e27777f26fabbf_3fba9d125588406c91ff67b52ec9f4c3/node_modules/@sveltejs/kit/src/runtime/app/env/index.js?v=5e96d286";
import { page } from "/node_modules/.pnpm/@sveltejs+kit@3.0.0_patch_hash=0d35d370d3e5fb6e0c801cd1079013b3d487d6e301e27777f26fabbf_3fba9d125588406c91ff67b52ec9f4c3/node_modules/@sveltejs/kit/src/runtime/app/state/index.js?v=5e96d286";
import ContactForms from "/src/routes/contact/ContactForms.svelte";
import SignIn from "/src/lib/SignIn.svelte";

var root = $.add_locations($.from_html(`<p data-testid="session-failed" class="svelte-12qhfyh"> </p>`), _layout[$.FILENAME], [[66, 4]]);
var root_1 = $.add_locations($.from_html(`<p data-testid="script-safe" class="svelte-12qhfyh"> </p>`), _layout[$.FILENAME], [[71, 2]]);

var root_2 = $.add_locations($.from_html(`<!> <nav data-testid="app-nav" class="svelte-12qhfyh"><a href="/" class="svelte-12qhfyh">Home</a> <a href="/about" class="svelte-12qhfyh">About</a> <a href="/items/42" class="svelte-12qhfyh">Item 42</a> <a href="/items/42?retain=1" class="svelte-12qhfyh">Item 42 retained</a> <a href="/todos" class="svelte-12qhfyh">Todos</a> <a href="/empty" class="svelte-12qhfyh">Empty</a> <a href="/api" class="svelte-12qhfyh">API</a> <a href="/fetch" class="svelte-12qhfyh">Server fetch</a> <a href="/request-fetch" class="svelte-12qhfyh">Request fetch</a> <a href="/nested-universal" class="svelte-12qhfyh">Nested fetch</a> <a href="/destinations" class="svelte-12qhfyh">Destinations</a> <a href="/pricing" class="svelte-12qhfyh">Pricing</a> <a href="/actions" class="svelte-12qhfyh">Actions</a> <a href="/docs/guide/getting-started" class="svelte-12qhfyh">Docs</a> <a href="/account" class="svelte-12qhfyh">Account</a> <a href="/stream" class="svelte-12qhfyh">Stream</a> <a href="/live" class="svelte-12qhfyh">Live</a> <a href="/batch" class="svelte-12qhfyh">Batch</a></nav> <!> <!> <main class="svelte-12qhfyh"><!> <!></main> <footer data-testid="deployment" class="svelte-12qhfyh"> </footer>`, 1), _layout[$.FILENAME], [
	[
		26,
		0,
		[
			[27, 2],
			[28, 2],
			[29, 2],
			[30, 2],
			[31, 2],
			[32, 2],
			[33, 2],
			[34, 2],
			[35, 2],
			[36, 2],
			[37, 2],
			[38, 2],
			[39, 2],
			[40, 2],
			[41, 2],
			[42, 2],
			[43, 2],
			[44, 2]
		]
	],
	[74, 0],
	[88, 0]
]);

function _layout($$anchor, $$props) {
	$.check_target(new.target);
	$.push($$props, true, _layout);

	/**
	 * The root layout is the one node in a branch that no `+error.svelte` can
	 * guard: kit wraps the root error page *inside* this layout rather than
	 * this layout inside it (`runtime/error-chain.js`), so the boundary around
	 * it has no `failed` snippet and a throw here has nowhere to go. That makes
	 * it the only place a page can be made to fail the way an engine failure
	 * fails, and /error/render is the fixture that does it.
	 */
	const failTheWholeDocument = () => {
		throw new Error("skgo: the root layout cannot render /error/render");
	};

	var $$exports = { ...$.legacy_api() };
	var fragment = root_2();
	var node = $.first_child(fragment);

	{
		var consequent = ($$anchor) => {
			var text = $.text();

			$.template_effect(($0) => $.set_text(text, $0), [() => failTheWholeDocument()]);
			$.append($$anchor, text);
		};

		$.add_svelte_meta(
			() => $.if(node, ($$render) => {
				if ($.strict_equals(page.route.id, "/error/render")) $$render(consequent);
			}),
			'if',
			_layout,
			22,
			0
		);
	}

	var node_1 = $.sibling(node, 4);

	{
		const failed = $.wrap_snippet(_layout, function ($$anchor, error = $.noop) {
			$.validate_snippet_args(...arguments);

			var p = root();
			var text_1 = $.only_child(p, true);

			$.template_effect(() => $.set_text(text_1, error().message));
			$.append($$anchor, p);
		});

		$.boundary(node_1, { failed }, ($$anchor) => {
			var fragment_2 = $.comment();
			var node_2 = $.first_child(fragment_2);

			{
				var consequent_1 = ($$anchor) => {
					var fragment_3 = $.comment();
					var node_3 = $.first_child(fragment_3);

					$.add_svelte_meta(() => SignIn(node_3, {}), 'component', _layout, 63, 4, { componentTag: 'SignIn' });
					$.append($$anchor, fragment_3);
				};

				$.add_svelte_meta(
					() => $.if(node_2, ($$render) => {
						if (!building) $$render(consequent_1);
					}),
					'if',
					_layout,
					62,
					2
				);
			}

			$.append($$anchor, fragment_2);
		});
	}

	var node_4 = $.sibling(node_1, 2);

	{
		var consequent_2 = ($$anchor) => {
			var p_1 = root_1();
			var text_2 = $.only_child(p_1, true);

			$.template_effect(() => $.set_text(text_2, $$props.data.scriptSafe));
			$.append($$anchor, p_1);
		};

		$.add_svelte_meta(
			() => $.if(node_4, ($$render) => {
				if ($$props.data.scriptSafe) $$render(consequent_2);
			}),
			'if',
			_layout,
			70,
			0
		);
	}

	var main = $.sibling(node_4, 2);
	var node_5 = $.child(main);

	$.add_svelte_meta(() => $.snippet(node_5, () => $$props.children), 'render', _layout, 75, 2);

	var node_6 = $.sibling(node_5, 2);

	{
		var consequent_3 = ($$anchor) => {
			var fragment_4 = $.comment();
			var node_7 = $.first_child(fragment_4);

			{
				let $0 = $.derived(() => $.strict_equals(page.route.id, "/contact"));

				$.add_svelte_meta(
					() => ContactForms(node_7, {
						get showInbox() {
							return $.get($0);
						}
					}),
					'component',
					_layout,
					78,
					4,
					{ componentTag: 'ContactForms' }
				);
			}

			$.append($$anchor, fragment_4);
		};

		$.add_svelte_meta(
			() => $.if(node_6, ($$render) => {
				if ($.strict_equals(page.route.id, "/contact") || $.strict_equals(page.route.id, "/items/[id]") || $.strict_equals(page.route.id, "/typed-load/[number=Order]")) $$render(consequent_3);
			}),
			'if',
			_layout,
			77,
			2
		);
	}

	$.reset(main);

	var footer = $.sibling(main, 2);
	var text_3 = $.only_child(footer, true);

	$.template_effect(() => $.set_text(text_3, $$props.data.deployment));
	$.append($$anchor, fragment);

	return $.pop($$exports);
}

if (import.meta.hot) {
	_layout = $.hmr(_layout);

	import.meta.hot.acceptExports(["default"],(module) => {
		$.cleanup_styles('svelte-12qhfyh');
		_layout[$.HMR].update(module.default);
	});
}

export default _layout;
import "/src/routes/+layout.svelte?svelte&type=style&lang.css";

//# sourceMappingURL=data:application/json;base64,eyJ2ZXJzaW9uIjozLCJtYXBwaW5ncyI6Ijs7Ozs7O0FBQ0UsTUFBTSxHQUFHLFFBQVEsUUFBUSxVQUFVO0FBQ25DLE1BQU0sR0FBRyxJQUFJLFFBQVEsWUFBWTtBQUNqQyxNQUFNLENBQUMsWUFBWSxNQUFNLCtCQUErQjtBQUN4RCxNQUFNLENBQUMsTUFBTSxNQUFNLG9CQUFvQjs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7Ozs7OztvQ0FKekMsQ0FBQzs7Ozs7Ozs7Ozs7O0NBZ0JDLEtBQUssQ0FBQyxvQkFBb0IsR0FBRyxNQUFNO0VBQ2pDLEtBQUssQ0FBQyxHQUFHLENBQUMsS0FBSyxDQUFDLG1EQUFtRCxDQUFDO0NBQ3RFLENBQUM7Ozs7Ozs7Ozs7MERBSUEsb0JBQW9CLEVBQUU7Ozs7Ozt3QkFEcEIsSUFBSSxDQUFDLEtBQUssQ0FBQyxFQUFFLEVBQUssZUFBZTs7Ozs7Ozs7Ozs7O1FBMkMxQixNQUFNLCtDQUFDLEtBQUs7OztPQUNuQixDQUFDOzZCQUFELENBQUM7OzhDQUErQixBQUFDLEtBQUssR0FBVyxPQUFPO3NCQUF4RCxDQUFDOzs7dUJBRE0sTUFBTTs7Ozs7Ozs7OzZCQUZiLE1BQU07Ozs7OztVQURKLENBQUMsUUFBUTs7Ozs7Ozs7Ozs7Ozs7Ozs7T0FTYixHQUFFLEFBQUQ7NkJBQUQsR0FBRSxBQUFEOzs4Q0FBNEIsYUFBSyxVQUFVO3NCQUE1QyxHQUFFLEFBQUQ7Ozs7O1FBREMsYUFBSyxVQUFVOzs7Ozs7Ozs7S0FJbkIsSUFBSTtzQkFBSixJQUFJOzs7Ozs7Ozs7Ozs7NkNBSXdCLElBQUksQ0FBQyxLQUFLLENBQUMsRUFBRSxFQUFLLFVBQVU7OztXQUFwRCxZQUFZOzs7Ozs7Ozs7Ozs7Ozs7Ozs7UUFEVixvQkFBSSxDQUFDLEtBQUssQ0FBQyxFQUFFLEVBQUssVUFBVSxxQkFBSSxJQUFJLENBQUMsS0FBSyxDQUFDLEVBQUUsRUFBSyxhQUFhLHFCQUFJLElBQUksQ0FBQyxLQUFLLENBQUMsRUFBRSxFQUFLLDRCQUE0Qjs7Ozs7Ozs7O1NBSHZILElBQUk7O0tBY0osTUFBTSxhQWROLElBQUk7MkJBY0osTUFBTTs7NENBQTJCLGFBQUssVUFBVTs7OztBQXBFekMiLCJuYW1lcyI6W10sImlnbm9yZUxpc3QiOltdLCJzb3VyY2VzIjpbIitsYXlvdXQuc3ZlbHRlIl0sInNvdXJjZXNDb250ZW50IjpbIjxzY3JpcHQgbGFuZz1cInRzXCI+XG4gIGltcG9ydCB7IGJ1aWxkaW5nIH0gZnJvbSBcIiRhcHAvZW52XCI7XG4gIGltcG9ydCB7IHBhZ2UgfSBmcm9tIFwiJGFwcC9zdGF0ZVwiO1xuICBpbXBvcnQgQ29udGFjdEZvcm1zIGZyb20gXCIuL2NvbnRhY3QvQ29udGFjdEZvcm1zLnN2ZWx0ZVwiO1xuICBpbXBvcnQgU2lnbkluIGZyb20gXCIjbGliL1NpZ25Jbi5zdmVsdGVcIjtcblxuICBsZXQgeyBkYXRhLCBjaGlsZHJlbiB9ID0gJHByb3BzKCk7XG5cbiAgLyoqXG4gICAqIFRoZSByb290IGxheW91dCBpcyB0aGUgb25lIG5vZGUgaW4gYSBicmFuY2ggdGhhdCBubyBgK2Vycm9yLnN2ZWx0ZWAgY2FuXG4gICAqIGd1YXJkOiBraXQgd3JhcHMgdGhlIHJvb3QgZXJyb3IgcGFnZSAqaW5zaWRlKiB0aGlzIGxheW91dCByYXRoZXIgdGhhblxuICAgKiB0aGlzIGxheW91dCBpbnNpZGUgaXQgKGBydW50aW1lL2Vycm9yLWNoYWluLmpzYCksIHNvIHRoZSBib3VuZGFyeSBhcm91bmRcbiAgICogaXQgaGFzIG5vIGBmYWlsZWRgIHNuaXBwZXQgYW5kIGEgdGhyb3cgaGVyZSBoYXMgbm93aGVyZSB0byBnby4gVGhhdCBtYWtlc1xuICAgKiBpdCB0aGUgb25seSBwbGFjZSBhIHBhZ2UgY2FuIGJlIG1hZGUgdG8gZmFpbCB0aGUgd2F5IGFuIGVuZ2luZSBmYWlsdXJlXG4gICAqIGZhaWxzLCBhbmQgL2Vycm9yL3JlbmRlciBpcyB0aGUgZml4dHVyZSB0aGF0IGRvZXMgaXQuXG4gICAqL1xuICBjb25zdCBmYWlsVGhlV2hvbGVEb2N1bWVudCA9ICgpID0+IHtcbiAgICB0aHJvdyBuZXcgRXJyb3IoXCJza2dvOiB0aGUgcm9vdCBsYXlvdXQgY2Fubm90IHJlbmRlciAvZXJyb3IvcmVuZGVyXCIpO1xuICB9O1xuPC9zY3JpcHQ+XG5cbnsjaWYgcGFnZS5yb3V0ZS5pZCA9PT0gXCIvZXJyb3IvcmVuZGVyXCJ9XG4gIHtmYWlsVGhlV2hvbGVEb2N1bWVudCgpfVxuey9pZn1cblxuPG5hdiBkYXRhLXRlc3RpZD1cImFwcC1uYXZcIj5cbiAgPGEgaHJlZj1cIi9cIj5Ib21lPC9hPlxuICA8YSBocmVmPVwiL2Fib3V0XCI+QWJvdXQ8L2E+XG4gIDxhIGhyZWY9XCIvaXRlbXMvNDJcIj5JdGVtIDQyPC9hPlxuICA8YSBocmVmPVwiL2l0ZW1zLzQyP3JldGFpbj0xXCI+SXRlbSA0MiByZXRhaW5lZDwvYT5cbiAgPGEgaHJlZj1cIi90b2Rvc1wiPlRvZG9zPC9hPlxuICA8YSBocmVmPVwiL2VtcHR5XCI+RW1wdHk8L2E+XG4gIDxhIGhyZWY9XCIvYXBpXCI+QVBJPC9hPlxuICA8YSBocmVmPVwiL2ZldGNoXCI+U2VydmVyIGZldGNoPC9hPlxuICA8YSBocmVmPVwiL3JlcXVlc3QtZmV0Y2hcIj5SZXF1ZXN0IGZldGNoPC9hPlxuICA8YSBocmVmPVwiL25lc3RlZC11bml2ZXJzYWxcIj5OZXN0ZWQgZmV0Y2g8L2E+XG4gIDxhIGhyZWY9XCIvZGVzdGluYXRpb25zXCI+RGVzdGluYXRpb25zPC9hPlxuICA8YSBocmVmPVwiL3ByaWNpbmdcIj5QcmljaW5nPC9hPlxuICA8YSBocmVmPVwiL2FjdGlvbnNcIj5BY3Rpb25zPC9hPlxuICA8YSBocmVmPVwiL2RvY3MvZ3VpZGUvZ2V0dGluZy1zdGFydGVkXCI+RG9jczwvYT5cbiAgPGEgaHJlZj1cIi9hY2NvdW50XCI+QWNjb3VudDwvYT5cbiAgPGEgaHJlZj1cIi9zdHJlYW1cIj5TdHJlYW08L2E+XG4gIDxhIGhyZWY9XCIvbGl2ZVwiPkxpdmU8L2E+XG4gIDxhIGhyZWY9XCIvYmF0Y2hcIj5CYXRjaDwvYT5cbjwvbmF2PlxuXG48IS0tXG5cdE5vIGBwZW5kaW5nYCBzbmlwcGV0LCBkZWxpYmVyYXRlbHkuIFN2ZWx0ZSdzIHNlcnZlciBjb21waWxlciBlbWl0cyB0aGVcblx0cGVuZGluZyBibG9jayAqaW5zdGVhZCBvZiogdGhlIGNoaWxkcmVuIG9mIGEgYm91bmRhcnkgdGhhdCBoYXMgb25lXG5cdChzdmVsdGUvc3JjL2NvbXBpbGVyL3BoYXNlcy8zLXRyYW5zZm9ybS9zZXJ2ZXIvdmlzaXRvcnMvU3ZlbHRlQm91bmRhcnkuanMpLFxuXHRzbyBhIGJvdW5kYXJ5IHdpdGggYSBwZW5kaW5nIHNuaXBwZXQgaXMgYSBib3VuZGFyeSB3aG9zZSBjb250ZW50cyBuZXZlclxuXHRyZWFjaCB0aGUgZG9jdW1lbnQg4oCUIGFuZCB0aGUgc2Vzc2lvbiB3b3VsZCB0aGVuIGJlIGZldGNoZWQgYWdhaW4gdGhlIG1vbWVudFxuXHR0aGUgcGFnZSBoeWRyYXRlZC4gVGhlIHNlc3Npb24gaXMgb24gZXZlcnkgcGFnZTsgaXQgYXJyaXZlcyB3aXRoIGl0LlxuLS0+XG48c3ZlbHRlOmJvdW5kYXJ5PlxuICA8IS0tXG5cdFx0YGJ1aWxkaW5nYCBpcyBraXQncyBvd24gYW5zd2VyIHRvIGEgcHJlcmVuZGVyZWQgcGFnZSB0aGF0IHdhbnRzXG5cdFx0cGVyLXJlcXVlc3QgZGF0YTogdGhlcmUgaXMgbm8gdmlzaXRvciB3aGlsZSB0aGUgYnVpbGQgcnVucywgc28gYSBwYWdlXG5cdFx0dGhlIGJ1aWxkIHdyaXRlcyB0byBkaXNrIHNob3dzIG5vIHNlc3Npb24gYW5kIHBpY2tzIG9uZSB1cCB3aGVuIGl0XG5cdFx0aHlkcmF0ZXMuIFdpdGhvdXQgaXQsIGtpdCByZWZ1c2VzIHRoZSBxdWVyeSBhbmQgdGhlIHByZXJlbmRlciBmYWlscy5cblx0LS0+XG4gIHsjaWYgIWJ1aWxkaW5nfVxuICAgIDxTaWduSW4gLz5cbiAgey9pZn1cbiAgeyNzbmlwcGV0IGZhaWxlZChlcnJvcil9XG4gICAgPHAgZGF0YS10ZXN0aWQ9XCJzZXNzaW9uLWZhaWxlZFwiPnsoZXJyb3IgYXMgRXJyb3IpLm1lc3NhZ2V9PC9wPlxuICB7L3NuaXBwZXR9XG48L3N2ZWx0ZTpib3VuZGFyeT5cblxueyNpZiBkYXRhLnNjcmlwdFNhZmV9XG4gIDxwIGRhdGEtdGVzdGlkPVwic2NyaXB0LXNhZmVcIj57ZGF0YS5zY3JpcHRTYWZlfTwvcD5cbnsvaWZ9XG5cbjxtYWluPlxuICB7QHJlbmRlciBjaGlsZHJlbigpfVxuICA8IS0tIEtlZXAgdGhlIHNoYXJlZCBrZXllZCBmb3JtJ3MgcmVhY3RpdmUgb3duZXIgYWxpdmUgYWNyb3NzIGNhbGxlciBjaGFuZ2VzLiAtLT5cbiAgeyNpZiBwYWdlLnJvdXRlLmlkID09PSBcIi9jb250YWN0XCIgfHwgcGFnZS5yb3V0ZS5pZCA9PT0gXCIvaXRlbXMvW2lkXVwiIHx8IHBhZ2Uucm91dGUuaWQgPT09IFwiL3R5cGVkLWxvYWQvW251bWJlcj1PcmRlcl1cIn1cbiAgICA8Q29udGFjdEZvcm1zIHNob3dJbmJveD17cGFnZS5yb3V0ZS5pZCA9PT0gXCIvY29udGFjdFwifSAvPlxuICB7L2lmfVxuPC9tYWluPlxuXG48IS0tXG5cdFRoZSByb290IGxheW91dCdzIG93biBsb2FkLCBvbiBldmVyeSBwYWdlIGluIHRoZSBhcHAuIEl0IGlzIHRoZSBvdXRlcm1vc3Rcblx0bm9kZSBpbiBldmVyeSBicmFuY2gsIHdoaWNoIG1ha2VzIGl0IHRoZSBvbmUgbG9hZCB3aG9zZSBmYWlsdXJlIG5vXG5cdGArZXJyb3Iuc3ZlbHRlYCBjYW4gY2F0Y2g6IGtpdCBhbnN3ZXJzIHRoYXQgd2l0aCB0aGUgc3RhdGljIGVycm9yLmh0bWwgdGhlXG5cdGJ1aWxkIGNhcnJpZXMuIGAvP2Jvb209cm9vdC1sYXlvdXRgIGlzIHRoZSBmaXh0dXJlLlxuLS0+XG48Zm9vdGVyIGRhdGEtdGVzdGlkPVwiZGVwbG95bWVudFwiPntkYXRhLmRlcGxveW1lbnR9PC9mb290ZXI+XG5cbjxzdHlsZT5cbiAgOmdsb2JhbCg6cm9vdCkge1xuICAgIC0taW5rOiAjMTMyNTFmO1xuICAgIC0tcGFwZXI6ICNmOGY2ZWY7XG4gICAgLS1ncmVlbjogIzE3NGYzZDtcbiAgICAtLWxpbWU6ICNjN2YzNmI7XG4gICAgLS1saW5lOiByZ2JhKDE5LCAzNywgMzEsIDAuMTYpO1xuICAgIGZvbnQtZmFtaWx5OlxuICAgICAgSW50ZXIsXG4gICAgICB1aS1zYW5zLXNlcmlmLFxuICAgICAgc3lzdGVtLXVpLFxuICAgICAgLWFwcGxlLXN5c3RlbSxcbiAgICAgIEJsaW5rTWFjU3lzdGVtRm9udCxcbiAgICAgIFwiU2Vnb2UgVUlcIixcbiAgICAgIHNhbnMtc2VyaWY7XG4gICAgY29sb3I6IHZhcigtLWluayk7XG4gICAgYmFja2dyb3VuZDogdmFyKC0tcGFwZXIpO1xuICB9XG4gIDpnbG9iYWwoKikge1xuICAgIGJveC1zaXppbmc6IGJvcmRlci1ib3g7XG4gIH1cbiAgOmdsb2JhbChib2R5KSB7XG4gICAgbWF4LXdpZHRoOiA3NnJlbTtcbiAgICBtYXJnaW46IDAgYXV0bztcbiAgICBwYWRkaW5nOiAwIDEuMjVyZW07XG4gIH1cbiAgbmF2IHtcbiAgICBkaXNwbGF5OiBmbGV4O1xuICAgIGdhcDogMC4yNXJlbTtcbiAgICBvdmVyZmxvdy14OiBhdXRvO1xuICAgIHBhZGRpbmc6IDFyZW0gMDtcbiAgICBib3JkZXItYm90dG9tOiAxcHggc29saWQgdmFyKC0tbGluZSk7XG4gIH1cbiAgbmF2IGEge1xuICAgIGZsZXg6IG5vbmU7XG4gICAgcGFkZGluZzogMC40NXJlbSAwLjY1cmVtO1xuICAgIGNvbG9yOiBpbmhlcml0O1xuICAgIGZvbnQtc2l6ZTogMC44cmVtO1xuICAgIGZvbnQtd2VpZ2h0OiA3MDA7XG4gICAgdGV4dC1kZWNvcmF0aW9uOiBub25lO1xuICB9XG4gIG5hdiBhOmhvdmVyIHtcbiAgICBiYWNrZ3JvdW5kOiB2YXIoLS1saW1lKTtcbiAgfVxuICBtYWluIHtcbiAgICBtaW4taGVpZ2h0OiBjYWxjKDEwMHZoIC0gOXJlbSk7XG4gICAgcGFkZGluZzogMnJlbSAwIDVyZW07XG4gIH1cbiAgOmdsb2JhbChmb290ZXJbZGF0YS10ZXN0aWQ9XCJkZXBsb3ltZW50XCJdKSB7XG4gICAgcGFkZGluZzogMS41cmVtIDA7XG4gICAgYm9yZGVyLXRvcDogMXB4IHNvbGlkIHZhcigtLWxpbmUpO1xuICAgIGNvbG9yOiAjNjg3NjcwO1xuICAgIGZvbnQ6XG4gICAgICA3MDAgMC43NXJlbSB1aS1tb25vc3BhY2UsXG4gICAgICBtb25vc3BhY2U7XG4gIH1cbiAgW2RhdGEtdGVzdGlkPVwic2NyaXB0LXNhZmVcIl0ge1xuICAgIG92ZXJmbG93LXdyYXA6IGFueXdoZXJlO1xuICAgIGZvbnQtZmFtaWx5OiB1aS1tb25vc3BhY2UsIG1vbm9zcGFjZTtcbiAgfVxuICA6Z2xvYmFsKGJ1dHRvbiksXG4gIDpnbG9iYWwoaW5wdXQpLFxuICA6Z2xvYmFsKHRleHRhcmVhKSB7XG4gICAgZm9udDogaW5oZXJpdDtcbiAgfVxuICA6Z2xvYmFsKGJ1dHRvbikge1xuICAgIGN1cnNvcjogcG9pbnRlcjtcbiAgfVxuPC9zdHlsZT5cbiJdLCJmaWxlIjoiL1VzZXJzL3R5bGVyLy5jb2RleC93b3JrdHJlZXMvNWYzNS9za2dvL2V4YW1wbGUvd2ViL3NyYy9yb3V0ZXMvK2xheW91dC5zdmVsdGUifQ==