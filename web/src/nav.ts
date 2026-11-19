// The shell's destination list. The rail, the breadcrumb, the keyboard
// shortcuts and the command palette are all generated from it, so adding a
// screen is one entry here plus its route in App.tsx.
import {
  Activity,
  Bot,
  BookOpen,
  Boxes,
  Database,
  FileCog,
  FlaskConical,
  KeyRound,
  LayoutDashboard,
  Network,
  ScrollText,
  Settings,
  ShieldCheck,
  Users,
  Building2,
  type LucideIcon,
} from 'lucide-react'

export type Section = 'Home' | 'Build' | 'Connect' | 'Govern' | 'Operate' | 'Studio'

export const SECTIONS: Section[] = ['Home', 'Build', 'Connect', 'Govern', 'Operate', 'Studio']

/** The catalogue key of a section's heading ("nav.section.build"). */
export function sectionKey(section: Section): string {
  return 'nav.section.' + section.toLowerCase()
}

export interface NavItem {
  id: string
  /** Catalogue key of the screen's name. */
  labelKey: string
  section: Section
  icon: LucideIcon
  path: string
  /** Alt+<digit> jumps here; browsers keep Ctrl+<digit> for their own tabs. */
  digit?: number
  /** Catalogue key of the command palette's one-line description. */
  hintKey: string
}

export const NAV: NavItem[] = [
  { id: 'overview', labelKey: 'nav.overview', section: 'Home', icon: LayoutDashboard, path: '/overview', digit: 1, hintKey: 'nav.overviewHint' },
  { id: 'agents', labelKey: 'nav.agents', section: 'Build', icon: Bot, path: '/agents', digit: 2, hintKey: 'nav.agentsHint' },
  { id: 'playground', labelKey: 'nav.playground', section: 'Build', icon: FlaskConical, path: '/playground', digit: 3, hintKey: 'nav.playgroundHint' },
  { id: 'channels', labelKey: 'nav.channels', section: 'Connect', icon: Network, path: '/channels', digit: 4, hintKey: 'nav.channelsHint' },
  { id: 'models', labelKey: 'nav.models', section: 'Connect', icon: Boxes, path: '/models', digit: 5, hintKey: 'nav.modelsHint' },
  { id: 'memory', labelKey: 'nav.memory', section: 'Connect', icon: Database, path: '/memory', hintKey: 'nav.memoryHint' },
  { id: 'credentials', labelKey: 'nav.credentials', section: 'Connect', icon: KeyRound, path: '/credentials', digit: 6, hintKey: 'nav.credentialsHint' },
  { id: 'safety', labelKey: 'nav.safety', section: 'Govern', icon: ShieldCheck, path: '/safety', digit: 7, hintKey: 'nav.safetyHint' },
  { id: 'users', labelKey: 'nav.users', section: 'Govern', icon: Users, path: '/users', digit: 8, hintKey: 'nav.usersHint' },
  { id: 'tenancy', labelKey: 'nav.tenancy', section: 'Govern', icon: Building2, path: '/tenancy', hintKey: 'nav.tenancyHint' },
  { id: 'usage', labelKey: 'nav.usage', section: 'Operate', icon: Activity, path: '/usage', hintKey: 'nav.usageHint' },
  { id: 'logs', labelKey: 'nav.logs', section: 'Operate', icon: ScrollText, path: '/logs', digit: 9, hintKey: 'nav.logsHint' },
  { id: 'config', labelKey: 'nav.config', section: 'Operate', icon: FileCog, path: '/config', hintKey: 'nav.configHint' },
  { id: 'docs', labelKey: 'nav.docs', section: 'Studio', icon: BookOpen, path: '/docs', hintKey: 'nav.docsHint' },
  { id: 'settings', labelKey: 'nav.settings', section: 'Studio', icon: Settings, path: '/settings', hintKey: 'nav.settingsHint' },
]

/** The destination a path belongs to (the builder highlights Agents). */
export function navForPath(pathname: string): NavItem | undefined {
  if (pathname === '/new-agent') return NAV.find((it) => it.id === 'agents')
  return NAV.find((it) => pathname === it.path || pathname.startsWith(it.path + '/'))
}

export const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform)

/** The label for the platform's command modifier. */
export const MOD = isMac ? '⌘' : 'Ctrl'
export const ALT = isMac ? '⌥' : 'Alt'

export function digitShortcut(n: number): string {
  return isMac ? `⌥${n}` : `Alt+${n}`
}
