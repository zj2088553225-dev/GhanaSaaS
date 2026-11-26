import request, { type ApiResponse } from '../utils/request'

export interface RevenueTrendItem {
  date: string
  amount: number
}

export type SKUSpecs = Record<string, string>

export interface LowStockSKU {
  id: number
  product_id: number
  product_name: string
  name: string
  specs: SKUSpecs
  sellable_stock: number
}

export interface DashboardSummary {
  today_orders: number
  today_revenue: number
  pending_orders: number
  revenue_trend: RevenueTrendItem[]
  low_stock_skus: LowStockSKU[]
}

export async function getDashboardSummary(): Promise<DashboardSummary> {
  const response = await request.get<ApiResponse<DashboardSummary>>('/admin/dashboard/summary')
  return response.data.data ?? {
    today_orders: 0,
    today_revenue: 0,
    pending_orders: 0,
    revenue_trend: [],
    low_stock_skus: [],
  }
}
