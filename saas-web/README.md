# Vue 3 + TypeScript + Vite

This template should help get you started developing with Vue 3 and TypeScript in Vite. The template uses Vue 3 `<script setup>` SFCs, check out the [script setup docs](https://v3.vuejs.org/api/sfc-script-setup.html#sfc-script-setup) to learn more.

Learn more about the recommended Project Setup and IDE Support in the [Vue Docs TypeScript Guide](https://vuejs.org/guide/typescript/overview.html#project-setup).

## Reservation feature flag

Reservations are disabled by default. Set `VITE_RESERVATION_ENABLED=true` only when the API uses
`RESERVATION_ENABLED=true` and its production OTP, Turnstile, and secret requirements are fully
configured. When false, reservation routes redirect home, storefront/admin navigation is omitted,
and the Web client does not call reservation APIs.
