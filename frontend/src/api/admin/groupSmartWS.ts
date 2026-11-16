import { apiClient } from '../client'

export interface SmartWSPolicy { enabled: boolean; revision: number }
export async function getSmartWS(id: number): Promise<SmartWSPolicy> {
  const { data } = await apiClient.get<SmartWSPolicy>(`/admin/groups/${id}/smart-ws`)
  return data
}
export async function setSmartWS(id: number, policy: SmartWSPolicy): Promise<SmartWSPolicy> {
  const { data } = await apiClient.put<SmartWSPolicy>(`/admin/groups/${id}/smart-ws`, policy)
  return data
}
