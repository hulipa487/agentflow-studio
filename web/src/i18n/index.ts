// The web app's i18n layer: three message catalogues, the current language,
// and t() to read a message.
//
// The catalogues are the shared files in ../../../i18n (one JSON file per
// namespace, per language), the same ones the desktop app embeds. English is
// the source: a key missing from another language falls back to it, and a key
// missing everywhere prints as the key, so a typo is visible on screen.
//
// t() is a plain function reading the current language, so it can be called
// from anywhere: a render, a toast, a store. Components pick up a language
// change because <App> subscribes to the store and re-renders the tree. The
// two things that do not re-render on their own must ask for the language
// explicitly with useLang(): a component wrapped in memo(), and a useMemo /
// useCallback whose result holds translated text (add the language to its
// dependencies). Module-level constants hold keys, never translated text.
import { create } from 'zustand'
import { formatMessage, type Params } from './format'

export type Lang = 'en' | 'zh-Hans' | 'zh-Hant'

/** The languages in toggle order, with the label each shows for itself. */
export const LANGS: { id: Lang; label: string; name: string }[] = [
  { id: 'en', label: 'EN', name: 'English' },
  { id: 'zh-Hans', label: '简体', name: '简体中文' },
  { id: 'zh-Hant', label: '繁體', name: '繁體中文' },
]

export type Catalogue = Record<string, string>

const files = import.meta.glob('../../../i18n/*/*.json', { import: 'default', eager: true }) as Record<string, Catalogue>

function load(): Record<Lang, Catalogue> {
  const out: Record<Lang, Catalogue> = { en: {}, 'zh-Hans': {}, 'zh-Hant': {} }
  for (const [path, messages] of Object.entries(files)) {
    const m = /\/i18n\/([^/]+)\/[^/]+\.json$/.exec(path)
    if (m && m[1] in out) Object.assign(out[m[1] as Lang], messages)
  }
  return out
}

export const CATALOGUES = load()

/** The catalogue files by "<lang>/<namespace>", for the catalogue checks. */
export function catalogueFiles(): Record<string, Catalogue> {
  const out: Record<string, Catalogue> = {}
  for (const [path, messages] of Object.entries(files)) {
    const m = /\/i18n\/([^/]+)\/([^/]+)\.json$/.exec(path)
    if (m) out[`${m[1]}/${m[2]}`] = messages
  }
  return out
}

const KEY = 'agentflow-studio:language'

function isLang(v: unknown): v is Lang {
  return v === 'en' || v === 'zh-Hans' || v === 'zh-Hant'
}

/**
 * The language for a BCP 47 tag, or null when Studio has none for it.
 * Traditional Chinese is the script tag Hant or, without a script, the regions
 * that write it (Taiwan, Hong Kong, Macau); any other Chinese is Simplified.
 */
export function matchLang(tag: string): Lang | null {
  const parts = tag.toLowerCase().replace(/_/g, '-').split('-')
  if (parts[0] === 'en') return 'en'
  if (parts[0] !== 'zh') return null
  if (parts.includes('hant')) return 'zh-Hant'
  if (parts.includes('hans')) return 'zh-Hans'
  return parts.some((p) => p === 'tw' || p === 'hk' || p === 'mo') ? 'zh-Hant' : 'zh-Hans'
}

/** The first of the browser's preferred languages Studio speaks, else English. */
export function detectLang(preferred: readonly string[]): Lang {
  for (const tag of preferred) {
    const lang = matchLang(tag)
    if (lang) return lang
  }
  return 'en'
}

function initialLang(): Lang {
  try {
    const saved = localStorage.getItem(KEY)
    if (isLang(saved)) return saved
  } catch {
    /* storage is unavailable: follow the browser */
  }
  if (typeof navigator === 'undefined') return 'en'
  return detectLang(navigator.languages?.length ? navigator.languages : [navigator.language ?? 'en'])
}

function apply(lang: Lang) {
  if (typeof document !== 'undefined') document.documentElement.lang = lang
}

interface I18nState {
  lang: Lang
  setLang: (lang: Lang) => void
}

export const useI18n = create<I18nState>((set) => ({
  lang: initialLang(),
  setLang: (lang) => {
    try {
      localStorage.setItem(KEY, lang)
    } catch {
      /* not persisted: the choice lasts for this tab */
    }
    apply(lang)
    set({ lang })
  },
}))

apply(useI18n.getState().lang)

/** The current language, re-rendering the caller when it changes. */
export function useLang(): Lang {
  return useI18n((s) => s.lang)
}

export function currentLang(): Lang {
  return useI18n.getState().lang
}

const pluralRules = new Map<Lang, Intl.PluralRules>()

function category(lang: Lang): (n: number) => string {
  let rules = pluralRules.get(lang)
  if (!rules) {
    rules = new Intl.PluralRules(lang)
    pluralRules.set(lang, rules)
  }
  return (n) => rules.select(n)
}

/** The message for key in lang, falling back to English, then to the key. */
export function message(lang: Lang, key: string): string {
  return CATALOGUES[lang][key] ?? CATALOGUES.en[key] ?? key
}

export function translate(lang: Lang, key: string, params?: Params): string {
  return formatMessage(message(lang, key), params, category(lang))
}

/** The message for key in the current language, with its parameters filled in. */
export function t(key: string, params?: Params): string {
  return translate(useI18n.getState().lang, key, params)
}

/** Whether a key exists (in English, the source). */
export function hasMessage(key: string): boolean {
  return key in CATALOGUES.en
}

export type { Params }
