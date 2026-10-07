import { ready } from './state.remote';
export const prerender=true;
export async function load({data,fetch}) {
 const response=await fetch('/options-api/custom');
 const early=response.headers.get('x-public');
 const body=await response.text();
 const late=await ready();
 return {...data,early,body,late};
}
