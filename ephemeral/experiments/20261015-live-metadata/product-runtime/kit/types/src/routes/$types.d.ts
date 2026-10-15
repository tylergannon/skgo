import type * as Kit from '@sveltejs/kit';
import { MatcherParam } from '@sveltejs/kit/params';

type Expand<T> = T extends infer O ? { [K in keyof O]: O[K] } : never;
type RouteParams = {  };
type RouteId = '/';
type MaybeWithVoid<T> = {} extends T ? T | void : T;
export type RequiredKeys<T> = { [K in keyof T]-?: {} extends { [P in K]: T[K] } ? never : K; }[keyof T];
type OutputDataShape<T> = MaybeWithVoid<Omit<App.PageData, RequiredKeys<T>> & Partial<Pick<App.PageData, keyof T & keyof App.PageData>> & Record<string, any>>
type EnsureDefined<T> = T extends null | undefined ? {} : T;
type OptionalUnion<U extends Record<string, any>, A extends keyof U = U extends U ? keyof U : never> = U extends unknown ? { [P in Exclude<A, keyof U>]?: never } & U : never;
/** @deprecated Use the `snapshot` helper from `$app/navigation` instead. */
export type Snapshot<T = any> = Kit.Snapshot<T>;
export type ErrorProps = { error: App.Error };
type PageParentData = EnsureDefined<LayoutData>;
type LayoutRouteId = RouteId | "/" | "/(marketing)/pricing" | "/about" | "/account" | "/account/orders" | "/account/reruns/[slug]" | "/account/statement" | "/actions" | "/actions/cross/receive" | "/actions/cross/send" | "/actions/default" | "/actions/default/saved" | "/actions/options/no-client" | "/actions/options/no-ssr" | "/actions/profiles/[profile]" | "/actions/signed-in" | "/api" | "/async-ssr/[mode]/[group]" | "/batch" | "/bits-ui" | "/console" | "/contact" | "/destinations" | "/docs/[...rest]" | "/empty" | "/error/boundary" | "/error/command" | "/error/expected" | "/error/redirect" | "/error/render" | "/error/server-only" | "/error/unexpected" | "/event-layout/[section]/[item]/one" | "/event-layout/[section]/[item]/two" | "/fetch" | "/invoices" | "/items/[id]" | "/live" | "/middleware" | "/middleware/[slug]" | "/nested-universal" | "/optional" | "/plain" | "/prerender/[slug]" | "/prerender-consumer" | "/render-paths" | "/request-fetch" | "/runes-class" | "/shadow/[slug]" | "/spa" | "/stream" | "/todos" | "/todos/[id]" | "/todos/gate" | "/todos/pair" | "/typed-dependencies/[a=Order]/[b=Order]/[ignored]" | "/typed-load/[number=Order]" | "/universal-fetch" | "/universal-fetch/no-script" | null
type LayoutParams = RouteParams & { slug?: string | undefined; profile?: string | undefined; mode?: string | undefined; group?: string | undefined; rest?: string | undefined; section?: string | undefined; item?: string | undefined; id?: string | undefined; a?: MatcherParam<(typeof import('../../../../../../../../example/web/src/params.ts').params)["Order"]> | undefined; b?: MatcherParam<(typeof import('../../../../../../../../example/web/src/params.ts').params)["Order"]> | undefined; ignored?: string | undefined; number?: MatcherParam<(typeof import('../../../../../../../../example/web/src/params.ts').params)["Order"]> | undefined }
type LayoutServerParentData = EnsureDefined<{}>;
type LayoutParentData = EnsureDefined<{}>;

export type PageServerData = null;
export type PageData = Expand<PageParentData>;
export type PageProps = { params: RouteParams; data: PageData }
export type LayoutServerLoad<OutputData extends OutputDataShape<LayoutServerParentData> = OutputDataShape<LayoutServerParentData>> = Kit.ServerLoad<LayoutParams, LayoutServerParentData, OutputData, LayoutRouteId>;
export type LayoutServerLoadEvent = Parameters<LayoutServerLoad>[0];
export type LayoutServerData = Expand<OptionalUnion<EnsureDefined<Kit.LoadProperties<Awaited<ReturnType<typeof import('../../../../../../../../example/web/src/routes/+layout.server.js').load>>>>>>;
export type LayoutData = Expand<Omit<LayoutParentData, keyof LayoutServerData> & EnsureDefined<LayoutServerData>>;
export type LayoutProps = { params: LayoutParams; data: LayoutData; children: import("svelte").Snippet }
export type RequestEvent = Kit.RequestEvent<RouteParams, RouteId>;