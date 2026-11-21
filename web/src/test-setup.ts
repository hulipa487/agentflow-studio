// Provide an in-memory Web Storage stub before any stores load so Node 25's
// unconfigured localStorage warning does not fire during tests.
const memoryStorage = (): Storage => {
  const store = new Map<string, string>()
  return {
    get length() {
      return store.size
    },
    clear() {
      store.clear()
    },
    getItem(key) {
      return store.get(key) ?? null
    },
    key(index) {
      return Array.from(store.keys())[index] ?? null
    },
    removeItem(key) {
      store.delete(key)
    },
    setItem(key, value) {
      store.set(key, String(value))
    },
  }
}

Object.defineProperty(globalThis, 'localStorage', { value: memoryStorage(), configurable: true, writable: true })
Object.defineProperty(globalThis, 'sessionStorage', { value: memoryStorage(), configurable: true, writable: true })

// The tests assert Studio's English text. The first language otherwise
// follows the machine's locale (navigator.languages, which Node fills from
// LANG), so a Chinese-locale machine would fail them: pin English.
import { useI18n } from '@/i18n'

useI18n.setState({ lang: 'en' })
