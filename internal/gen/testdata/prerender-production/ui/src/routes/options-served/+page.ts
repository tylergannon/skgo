export async function load({fetch}) {const response=await fetch('/options-live-api');return {body:await response.text()};}
