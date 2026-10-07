export const prerender=true;
export async function load({fetch}) {const response=await fetch('/options-api/default');return {body:await response.text()};}
