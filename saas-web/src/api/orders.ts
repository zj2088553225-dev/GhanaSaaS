import request, { type ApiResponse } from '../utils/request'

export type OrderStatus = 'pending' | 'processing' | 'shipped' | 'completed'
export type FulfillmentMethod = 'pickup' | 'delivery'
export type PaymentStatus = 'unpaid' | 'paid' | 'refunded'

export interface CheckoutItem {
  sku_id: number
  quantity: number
}

export interface CheckoutPayload {
  contact_name: string
  contact_phone: string
  address: string
  fulfillment_method: FulfillmentMethod
  items: CheckoutItem[]
}

export interface OrderSKU {
  id: number
  name: string
  erp_sku_code: string
  barcode: string
  location: string
  specs?: Record<string, string>
}

export interface OrderProduct {
  id: number
  name: string
  main_image: string
}

export interface OrderItem {
  id: number
  order_id: number
  sku_id: number
  product_name: string
  price: number
  quantity: number
  sku: OrderSKU
  product?: OrderProduct
  created_at: string
  updated_at: string
}

export interface OrderUser {
  id: number
  phone: string
}

export interface OrderCustomer {
  id: number
  phone?: string
  email?: string
  nickname: string
}

export interface Order {
  id: number
  order_no: string
  user_id: number
  user: OrderUser
  customer_id: number
  customer?: OrderCustomer
  tenant_id: number
  total_amount: number
  status: OrderStatus
  fulfillment_method: FulfillmentMethod
  tracking_number: string
  payment_status: PaymentStatus
  contact_name: string
  contact_phone: string
  address: string
  items: OrderItem[]
  created_at: string
  updated_at: string
}

export interface AdminOrderQueryParams {
  order_no?: string
  phone?: string
  status?: OrderStatus
  start_date?: string
  end_date?: string
  page?: number
  page_size?: number
  export?: boolean
  no_page?: boolean
}

export interface AdminOrderPage {
  items: Order[]
  total: number
  page: number
  page_size: number
}

export interface AdminOrderStatusPayload {
  status: OrderStatus
}

export interface AdminOrderShipPayload {
  tracking_number?: string
}

export async function checkout(payload: CheckoutPayload): Promise<Order> {
  const response = await request.post<ApiResponse<Order>>('/storefront/orders/checkout', payload)
  if (!response.data.data) {
    throw new Error('Checkout failed')
  }
  return response.data.data
}

export async function getStorefrontOrders(): Promise<Order[]> {
  const response = await request.get<ApiResponse<Order[]>>('/storefront/orders')
  return response.data.data ?? []
}

export async function getAdminOrders(params: AdminOrderQueryParams = {}): Promise<AdminOrderPage> {
  const response = await request.get<ApiResponse<AdminOrderPage>>('/admin/orders', { params })
  if (!response.data.data) {
    return { items: [], total: 0, page: params.page ?? 1, page_size: params.page_size ?? 20 }
  }
  return response.data.data
}

export async function updateAdminOrderStatus(id: number, status: OrderStatus): Promise<Order> {
  const response = await request.patch<ApiResponse<Order>>(`/admin/orders/${id}/status`, { status })
  if (!response.data.data) {
    throw new Error('Order status update failed')
  }
  return response.data.data
}

export async function shipAdminOrder(id: number, payload: AdminOrderShipPayload = {}): Promise<Order> {
  const response = await request.post<ApiResponse<Order>>(`/admin/orders/${id}/ship`, payload)
  if (!response.data.data) {
    throw new Error('Order shipment failed')
  }
  return response.data.data
}

export async function markAdminOrderPaid(id: number): Promise<Order> {
  const response = await request.post<ApiResponse<Order>>(`/admin/orders/${id}/pay`)
  if (!response.data.data) {
    throw new Error('Order payment update failed')
  }
  return response.data.data
}
