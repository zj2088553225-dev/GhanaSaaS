import request, { type ApiResponse } from '../utils/request'

export interface GlobalDashboardSummary {
  total_tenants: number
  total_revenue: number
  total_orders: number
  active_users: number
}

export async function getGlobalDashboardSummary(): Promise<GlobalDashboardSummary> {
  const response = await request.get<ApiResponse<GlobalDashboardSummary>>('/super/dashboard/summary')
  return response.data.data ?? {
    total_tenants: 0,
    total_revenue: 0,
    total_orders: 0,
    active_users: 0,
  }
}
