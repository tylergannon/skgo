export const prerender=true;
export async function load({fetch}) {const response=await fetch('/options-api/rejected');return {denied:response.headers.get('x-denied')};}
