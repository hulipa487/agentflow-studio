// The tests assert Studio's English text. The first language otherwise
// follows the machine's locale (navigator.languages, which Node fills from
// LANG), so a Chinese-locale machine would fail them: pin English.
import { useI18n } from '@/i18n'

useI18n.setState({ lang: 'en' })
