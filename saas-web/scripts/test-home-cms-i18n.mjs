import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const sitePagesApi = readFileSync(new URL('../src/api/sitePages.ts', import.meta.url), 'utf8')
const homeEditor = readFileSync(new URL('../src/views/admin/SitePageHomeEditor.vue', import.meta.url), 'utf8')
const sitePageController = readFileSync(new URL('../../internal/controller/site_page_controller.go', import.meta.url), 'utf8')
const sitePageService = readFileSync(new URL('../../internal/service/site_page_service.go', import.meta.url), 'utf8')
const sitePageRepository = readFileSync(new URL('../../internal/repository/site_page_repository.go', import.meta.url), 'utf8')
const sitePageTranslationHelpers = readFileSync(new URL('../../internal/service/site_page_translation_helpers.go', import.meta.url), 'utf8')
const contentTranslationService = readFileSync(new URL('../../internal/service/content_translation_service.go', import.meta.url), 'utf8')

assert.match(sitePagesApi, /SitePageTranslations/)
assert.match(sitePagesApi, /translations\?:\s*SitePageTranslations/)
assert.match(sitePagesApi, /saveHomeSitePageDraft\(title:\s*string,\s*content:\s*SitePageContent,\s*translations\?:\s*SitePageTranslations/)

assert.match(homeEditor, /adminTranslations\.english/)
assert.match(homeEditor, /adminTranslations\.chinese/)
assert.match(homeEditor, /zhBlocks/)
assert.match(homeEditor, /syncZhBlocksWithEnglish/)
assert.match(homeEditor, /mergeLocalizedCmsNode/)
assert.match(homeEditor, /translatableCmsFields/)
assert.match(homeEditor, /draft_content:\s*zhContentForSave\(englishContent\)/)

assert.match(sitePageController, /Translations\s+\*sitePageTranslationsPayload/)
assert.match(sitePageController, /DraftContent\s+map\[string\]any\s+`json:"draft_content"`/)
assert.match(sitePageService, /Translations\s+\*SitePageDraftTranslationsInput/)
assert.match(sitePageService, /sitePageTranslationContent\(ctx,\s*page\.ID,\s*TranslationFieldPublishedContent/)
assert.match(sitePageService, /s\.repo\.SaveDraftAtomic\(ctx,/)
assert.match(sitePageService, /s\.repo\.PublishAtomic\(ctx,/)
assert.doesNotMatch(sitePageService, /saveHomeDraftTranslations|publishHomeTranslations|SaveTranslationsForEntity\(/, 'CMS writes must not save translations after the page transaction')
assert.match(sitePageRepository, /func \(r \*SitePageRepository\) SaveDraftAtomic[\s\S]*Transaction\(func\(tx \*gorm\.DB\) error[\s\S]*saveSitePageTranslationMutation\(tx,/)
assert.match(sitePageRepository, /func \(r \*SitePageRepository\) PublishAtomic[\s\S]*Transaction\(func\(tx \*gorm\.DB\) error[\s\S]*field_key = \? AND locale = \?[\s\S]*saveSitePageTranslationMutation\(tx,/)
assert.match(sitePageTranslationHelpers, /mergeLocalizedSitePageContent/)
assert.match(sitePageTranslationHelpers, /sitePageTranslationContentFromBase/)
assert.match(sitePageTranslationHelpers, /sitePageTranslatableTextFields/)

assert.match(contentTranslationService, /TranslationEntitySitePage\s*=\s*"site_page"/)
assert.match(contentTranslationService, /TranslationFieldDraftContent\s*=\s*"draft_content"/)
assert.match(contentTranslationService, /TranslationFieldPublishedContent\s*=\s*"published_content"/)
assert.match(contentTranslationService, /TranslationEntitySitePage:\s*\{[\s\S]*TranslationFieldDraftContent:\s*translationFieldJSON/)

console.log('home CMS i18n tests passed')
