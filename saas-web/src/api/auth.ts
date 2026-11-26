import request, { type ApiResponse } from '../utils/request'
import type { AuthUser } from '../stores/auth'
import type { CustomerProfile } from '../stores/customerAuth'

export interface LoginRequest {
  phone: string
  password: string
}

export interface CustomerLoginRequest {
  email: string
  password: string
}

export interface LoginResult {
  token: string
  user: AuthUser
}

export interface CustomerLoginResult {
  token: string
  user: CustomerProfile
}

export interface RegisterCustomerRequest {
  email: string
  password: string
}

export interface RegisterCustomerResult {
  user: CustomerProfile
}

export async function login(payload: LoginRequest): Promise<LoginResult> {
  const response = await request.post<ApiResponse<LoginResult>>('/auth/login', payload)
  if (!response.data.data) {
    throw new Error('Login failed')
  }
  return response.data.data
}

export async function loginCustomer(payload: CustomerLoginRequest): Promise<CustomerLoginResult> {
  const response = await request.post<ApiResponse<CustomerLoginResult>>('/auth/login', payload)
  if (!response.data.data) {
    throw new Error('Login failed')
  }
  return response.data.data
}

export async function registerCustomer(payload: RegisterCustomerRequest): Promise<RegisterCustomerResult> {
  const response = await request.post<ApiResponse<RegisterCustomerResult>>('/auth/register', payload)
  if (!response.data.data) {
    throw new Error('Register failed')
  }
  return response.data.data
}

export async function getCurrentCustomer(): Promise<CustomerProfile> {
  const response = await request.get<ApiResponse<CustomerProfile>>('/auth/me')
  if (!response.data.data) {
    throw new Error('Customer not found')
  }
  return response.data.data
}

export async function logoutCustomer(): Promise<void> {
  await request.post<ApiResponse<never>>('/auth/logout')
}

export async function requestPasswordReset(email: string): Promise<string> {
  const response = await request.post<ApiResponse<never>>('/auth/password/request', { email })
  return response.data.message || 'If the email exists, reset instructions will be sent.'
}

export async function resetPassword(token: string, newPassword: string): Promise<string> {
  const response = await request.post<ApiResponse<never>>('/auth/password/reset', {
    token,
    new_password: newPassword,
  })
  return response.data.message || 'Password reset successful. You can now log in.'
}
