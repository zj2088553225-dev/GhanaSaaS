import request, { type ApiResponse } from '../utils/request'

export interface Category {
  id: number
  tenant_id: number
  parent_id: number | null
  name: string
  sort: number
  icon: string
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface CategoryPayload {
  parent_id?: number | null
  name: string
  sort: number
  icon: string
  is_active: boolean
}

export async function getCategories(): Promise<Category[]> {
  const response = await request.get<ApiResponse<Category[]>>('/categories')
  return response.data.data ?? []
}

export async function createCategory(payload: CategoryPayload): Promise<Category> {
  const response = await request.post<ApiResponse<Category>>('/categories', payload)
  if (!response.data.data) {
    throw new Error('店铺分类创建失败')
  }
  return response.data.data
}

export async function updateCategory(id: number, payload: CategoryPayload): Promise<Category> {
  const response = await request.put<ApiResponse<Category>>(`/categories/${id}`, payload)
  if (!response.data.data) {
    throw new Error('店铺分类更新失败')
  }
  return response.data.data
}

export async function deleteCategory(id: number): Promise<void> {
  await request.delete(`/categories/${id}`)
}
