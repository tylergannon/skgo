import {prerender} from '$app/server';
import {appendFileSync} from 'node:fs';
export const catalog=prerender('unchecked',(name:string)=>{appendFileSync('async.log','body:'+name+'\n');return 'build:'+name;},{inputs:async()=>{appendFileSync('async.log','inputs:start\n');await new Promise(resolve=>setTimeout(resolve,500));appendFileSync('async.log','inputs:end\n');return ['atlas','beacon'];}});