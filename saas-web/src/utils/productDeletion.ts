export const PRODUCT_HAS_ORDER_RECORDS_MESSAGE = '该商品已有订单记录，不能删除，请下架或归档'
export const PRODUCT_DELETE_FAILED_MESSAGE = '商品删除失败，请稍后重试'

export function productDeletionErrorMessage(error: unknown): string {
  const message = error instanceof Error ? error.message : ''
  if (message.includes(PRODUCT_HAS_ORDER_RECORDS_MESSAGE)) {
    return PRODUCT_HAS_ORDER_RECORDS_MESSAGE
  }
  if (!message || /SQLSTATE|violates foreign key constraint|constraint/i.test(message)) {
    return PRODUCT_DELETE_FAILED_MESSAGE
  }
  return message
}
