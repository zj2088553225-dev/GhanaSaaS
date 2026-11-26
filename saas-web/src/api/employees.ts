import request, { type ApiResponse } from '../utils/request'

export type EmployeeRole = 'finance' | 'picker'

export interface Employee {
  id: number
  tenant_id?: number
  role: EmployeeRole
  phone: string
  status: string
  created_at: string
  updated_at: string
}

export interface CreateEmployeePayload {
  phone: string
  password: string
  role: EmployeeRole
}

export async function getEmployees(): Promise<Employee[]> {
  const response = await request.get<ApiResponse<Employee[]>>('/admin/employees')
  return response.data.data ?? []
}

export async function createEmployee(payload: CreateEmployeePayload): Promise<Employee> {
  const response = await request.post<ApiResponse<Employee>>('/admin/employees', payload)
  if (!response.data.data) {
    throw new Error('Employee creation failed')
  }
  return response.data.data
}
