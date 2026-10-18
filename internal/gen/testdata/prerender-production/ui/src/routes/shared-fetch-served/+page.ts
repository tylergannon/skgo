export const prerender=false;
export async function load({data,fetch}){const response=await fetch('/shared-alias/nested');return {...data,kitBody:await response.text(),kitHeader:response.headers.get('x-config-hook'),defaultHeader:response.headers.get('x-default')};}
