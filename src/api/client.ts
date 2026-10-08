export interface Group { id:number; name:string; description:string; created_at:string }
export interface User { id:number; username:string; display_name:string; role:'admin'|'user'; disabled:boolean; groups:Group[]; created_at:string }
export interface Session { user:User|null; csrf_token:string }
export interface ToolAccess { id:string; name:string; description:string; emoji:string; category:string; tags:string[]; can_use:boolean }
export interface GroupRule { group_id:number; visible:boolean|null; use:boolean|null }
export interface ToolPolicy extends Omit<ToolAccess,'can_use'> { enabled:boolean; guest_visible:boolean; guest_use:boolean; member_visible:boolean; member_use:boolean; group_rules:GroupRule[] }
export interface Audit { id:number; actor:string; action:string; target:string; created_at:string }
export class ApiError extends Error { constructor(public readonly status:number,message:string){super(message);this.name='ApiError'} }
let csrf='';
export async function api<T>(path:string, init:RequestInit={}) :Promise<T> {
 const headers=new Headers(init.headers); if(init.body!==undefined) headers.set('Content-Type','application/json');
 if(!['GET','HEAD'].includes((init.method||'GET').toUpperCase()) && csrf) headers.set('X-CSRF-Token',csrf);
 const response=await fetch(`/api${path}`,{...init,headers,credentials:'same-origin'});
 if(response.status===204) return undefined as T;
 const data=await response.json().catch(()=>null);
 if(!response.ok) throw new ApiError(response.status,typeof data?.error==='string'?data.error:`请求失败 (${response.status})`);
 if(path==='/session' || path==='/auth/login' || path==='/auth/logout') csrf=data.csrf_token;
 return data as T;
}
export const json=(method:string,body:unknown):RequestInit=>({method,body:JSON.stringify(body)});
