import {failedQuery as failed} from '../shared-error/policy.remote';
export const prerender=false;
export async function load(){try{await failed();throw new Error('remote failed to reject');}catch(error){const body=error.body??error;return {message:body.message,supportId:body.supportId,status:body.status,hookCalls:body.hookCalls};}}
