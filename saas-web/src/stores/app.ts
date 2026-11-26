import { defineStore } from 'pinia'

export const useAppStore = defineStore('app', {
  state: () => ({
    mobileMenuOpen: false,
  }),
  actions: {
    toggleMobileMenu() {
      this.mobileMenuOpen = !this.mobileMenuOpen
    },
    closeMobileMenu() {
      this.mobileMenuOpen = false
    },
  },
})
