import { apiClient } from '../client'
export interface ProxyBindingPolicy {enabled:boolean;only_twofa:boolean;platforms:string[];history_limit:number;auto_assign:boolean}
export interface ProxyBindingSummary {proxy_id:number;history:number;current:number;reserved:number;limit:number;extra_allowed:boolean;exit_verified:boolean;exit_ip?:string}
export async function getProxyBindingPolicy():Promise<ProxyBindingPolicy>{return (await apiClient.get('/admin/proxies/binding-policy')).data}
export async function saveProxyBindingPolicy(p:ProxyBindingPolicy):Promise<ProxyBindingPolicy>{return (await apiClient.put('/admin/proxies/binding-policy',p)).data}
export async function getProxyBindingSummaries():Promise<ProxyBindingSummary[]>{return (await apiClient.get('/admin/proxies/binding-summary')).data}
export async function allowOneProxyBinding(id:number):Promise<void>{await apiClient.post(`/admin/proxies/${id}/binding-allowance`,{confirm:true})}
export function matchesProxyBinding(p:ProxyBindingPolicy|null,platform:string,type:string,source:string):boolean{return !!p?.enabled && type==='oauth' && p.platforms.includes(platform) && (!p.only_twofa || source==='twofa')}

export interface OAuthReservation {id:string;proxy_id:number|null;auto_assigned:boolean}
export async function reserveOAuthImport(platform:string,proxy_id:number|null):Promise<OAuthReservation>{return (await apiClient.post('/admin/proxies/import-reservation',{platform,proxy_id})).data}
export async function releaseOAuthImport(ticket:string):Promise<void>{await apiClient.delete(`/admin/proxies/import-reservation/${encodeURIComponent(ticket)}`)}
