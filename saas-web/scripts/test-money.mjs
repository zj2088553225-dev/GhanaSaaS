import assert from 'node:assert/strict'
import { formatMoney } from '../src/utils/money.ts'

assert.equal(formatMoney(8000), 'GHS 80.00')
assert.equal(formatMoney(0), 'GHS 0.00')
assert.equal(formatMoney(8050), 'GHS 80.50')
