import { readFileSync } from 'node:fs'
import path from 'node:path'

import { describe, expect, test } from 'vitest'

const indexHtml = readFileSync(path.resolve('index.html'), 'utf8')
const robots = readFileSync(path.resolve('public/robots.txt'), 'utf8')
const sitemap = readFileSync(path.resolve('public/sitemap.xml'), 'utf8')

describe('public search metadata', () => {
  test('presents ZeusZS as the primary search identity', () => {
    const document = new DOMParser().parseFromString(indexHtml, 'text/html')

    expect(document.title).toMatch(/^宙斯智算 ZEUSZS/)
    expect(
      document.querySelector<HTMLMetaElement>('meta[name="description"]')
        ?.content
    ).toContain('宙斯智算 ZEUSZS')
    expect(
      document.querySelector<HTMLLinkElement>('link[rel="canonical"]')?.href
    ).toBe('https://zeuszs.ai/')
    const searchShell = document.querySelector('[data-seo-shell]')
    expect(searchShell?.textContent).toContain('宙斯智算')
    expect(searchShell?.textContent).toContain('宙斯智算（上海）科技有限公司')
  })

  test('publishes valid organization and website structured data', () => {
    const document = new DOMParser().parseFromString(indexHtml, 'text/html')
    const script = document.querySelector<HTMLScriptElement>(
      'script[type="application/ld+json"]'
    )
    const structuredData = JSON.parse(script?.textContent ?? '{}') as {
      '@graph'?: Array<Record<string, unknown>>
    }

    expect(structuredData['@graph']).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          '@type': 'Organization',
          name: '宙斯智算（上海）科技有限公司',
          url: 'https://zeuszs.ai/',
        }),
        expect.objectContaining({
          '@type': 'WebSite',
          name: '宙斯智算',
          url: 'https://zeuszs.ai/',
        }),
      ])
    )
  })

  test('advertises the sitemap and only lists public brand pages', () => {
    expect(robots).toContain('Sitemap: https://zeuszs.ai/sitemap.xml')
    expect(robots).toContain('Disallow: /api/')
    expect(sitemap).toContain('<loc>https://zeuszs.ai/</loc>')
    expect(sitemap).toContain('<loc>https://zeuszs.ai/about</loc>')
    expect(sitemap).not.toContain('/dashboard')
  })
})
