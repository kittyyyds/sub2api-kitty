import { apiClient } from '../client'
export interface IQResult { id:number; verdict:'correct'|'other'|'error'; answer:string; error?:string; model:string; reasoning:string; finished_at:string }
export interface IQSummary { account_id:number;plan_id:number;enabled:boolean;model:string;reasoning:string;interval_seconds:number;running_until:string|null;next_run_at:string|null;results:IQResult[] }
export async function getIQSummaries(ids:number[]):Promise<IQSummary[]> {if(!ids.length)return [];return (await apiClient.post<IQSummary[]>('/admin/accounts/iq-summaries',{account_ids:ids})).data}
