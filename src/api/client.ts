export interface Group { id:number; name:string; description:string; created_at:string }
export interface Category { id:number; name:string; description:string; sort:number; created_at:string }
export interface User { id:number; username:string; display_name:string; role:'admin'|'user'; disabled:boolean; groups:Group[]; created_at:string }
export interface Session { user:User|null; csrf_token:string }
export interface ToolAccess { id:string; name:string; description:string; emoji:string; category:string; tags:string[]; can_use:boolean }
export interface GroupRule { group_id:number; visible:boolean|null; use:boolean|null }
export interface ToolPolicy extends Omit<ToolAccess,'can_use'> {
  category_id:number|null;
  sort:number;
  enabled:boolean;
  guest_visible:boolean;
  guest_use:boolean;
  member_visible:boolean;
  member_use:boolean;
  group_rules:GroupRule[];
}
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

const toolSlug=/^[a-z0-9][a-z0-9-]{0,63}$/;
/** 工具后端接口：拼到 /api/tools/<id><path>，复用统一鉴权与 CSRF。仅 catalog 中 backend:true 的工具使用。 */
export function toolApi<T>(id:string, path:string, init:RequestInit={}):Promise<T> {
 if(!toolSlug.test(id)||!path.startsWith('/')) throw new Error('工具接口路径无效');
 return api<T>(`/tools/${id}${path}`,init);
}
