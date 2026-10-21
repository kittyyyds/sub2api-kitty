import { apiClient } from '../client'
import type { IQBootstrap } from './autoConfig'
export interface IQLifecycle { quarantine_pending?:boolean; config:IQBootstrap;correct_streak:number;answer29_streak:number;passed_at?:string;quarantined_at?:string;action_error?:string }
export interface IQResult { id:number; verdict:'correct'|'other'|'error'|'waiting'; answer:string; error?:string; model:string; reasoning:string; finished_at:string }
export interface IQSummary { lifecycle?:IQLifecycle; account_id:number;plan_id:number;enabled:boolean;model:string;reasoning:string;interval_seconds:number;running_until:string|null;next_run_at:string|null;results:IQResult[] }
export async function getIQSummaries(ids:number[]):Promise<IQSummary[]> {if(!ids.length)return [];return (await apiClient.post<IQSummary[]>('/admin/accounts/iq-summaries',{account_ids:ids})).data}

export async function setIQQuarantineTarget(planId:number,groupId:number){await apiClient.put(`/admin/scheduled-test-plans/${planId}/iq-quarantine-target`,{group_id:groupId})}
