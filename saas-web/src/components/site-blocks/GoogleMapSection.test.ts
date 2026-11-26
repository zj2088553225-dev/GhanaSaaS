// @vitest-environment jsdom

import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { SitePageBlock } from '../../api/sitePages'
import GoogleMapSection from './GoogleMapSection.vue'

const i18n = createI18n({
  legacy: false,
  locale: 'en',
  messages: {
    en: {
      homeBlocks: {
        mapLocation: 'Location',
        mapEmbedTitle: 'Google Maps location',
        openInGoogleMaps: 'Open in Google Maps',
        mapConfigurationRequired: 'Map configuration required',
        mapMissingApiKey: 'Set VITE_GOOGLE_MAPS_EMBED_KEY.',
        mapMissingPlace: 'Set a place.',
      },
    },
  },
})

function mapBlock(overrides: Partial<SitePageBlock> = {}): SitePageBlock {
  return {
    type: 'map_section',
    enabled: true,
    title_en: 'Visit Us',
    title_zh: '到访我们',
    place_query: 'Accra, Ghana',
    map_height: 320,
    ...overrides,
  }
}

afterEach(() => {
  vi.unstubAllEnvs()
})

describe('GoogleMapSection', () => {
  it('shows a recognizable configuration prompt instead of an empty iframe when the key is missing', () => {
    vi.stubEnv('VITE_GOOGLE_MAPS_EMBED_KEY', '')
    const wrapper = mount(GoogleMapSection, {
      props: { block: mapBlock(), locale: 'en' },
      global: { plugins: [i18n] },
    })

    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(wrapper.get('[data-testid="google-map-config-hint"]').text()).toContain('VITE_GOOGLE_MAPS_EMBED_KEY')
  })

  it('renders the Embed API iframe with the required attributes and reacts to CMS place changes', async () => {
    vi.stubEnv('VITE_GOOGLE_MAPS_EMBED_KEY', 'test-public-key')
    const wrapper = mount(GoogleMapSection, {
      props: { block: mapBlock({ place_id: 'ChIJ-Test-ID' }), locale: 'zh' },
      global: { plugins: [i18n] },
    })

    const iframe = wrapper.get('iframe')
    const iframeURL = new URL(iframe.attributes('src') || '')
    expect(iframeURL.pathname).toBe('/maps/embed/v1/place')
    expect(iframeURL.searchParams.get('q')).toBe('place_id:ChIJ-Test-ID')
    expect(iframe.attributes('loading')).toBe('lazy')
    expect(iframe.attributes('allowfullscreen')).toBe('')
    expect(iframe.attributes('referrerpolicy')).toBe('strict-origin-when-cross-origin')
    expect(iframe.attributes('title')).toBe('到访我们')
    expect(wrapper.get('.map-embed-frame').classes()).toEqual(expect.arrayContaining(['w-full', 'max-w-full', 'overflow-hidden']))

    await wrapper.setProps({
      block: mapBlock({ place_query: 'Kumasi, Ghana', place_id: '' }),
      locale: 'en',
    })
    const updatedURL = new URL(wrapper.get('iframe').attributes('src') || '')
    expect(updatedURL.searchParams.get('q')).toBe('Kumasi, Ghana')
    expect(wrapper.get('iframe').attributes('title')).toBe('Visit Us')
  })
})
