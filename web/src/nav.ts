// The shell's destination list. The rail, the breadcrumb, the keyboard
// shortcuts and the command palette are all generated from it, so adding a
// screen is one entry here plus its route in App.tsx.
import {
  Activity,
  Bot,
  BookOpen,
  Boxes,
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

export interface NavItem {
  id: string
  label: string
  section: Section
  icon: LucideIcon
  path: string
  /** Alt+<digit> jumps here; browsers keep Ctrl+<digit> for their own tabs. */
  digit?: number
  /** The command palette's one-line description. */
  hint: string
}

export const NAV: NavItem[] = [
  { id: 'overview', label: 'Overview', section: 'Home', icon: LayoutDashboard, path: '/overview', digit: 1, hint: 'Instance health, live sessions, agents, channels and counters' },
  { id: 'agents', label: 'Agents', section: 'Build', icon: Bot, path: '/agents', digit: 2, hint: 'Configured agents; create, edit, apply and delete' },
  { id: 'playground', label: 'Playground', section: 'Build', icon: FlaskConical, path: '/playground', digit: 3, hint: 'Chat with an agent and watch the live trace' },
  { id: 'channels', label: 'Channels', section: 'Connect', icon: Network, path: '/channels', digit: 4, hint: 'Webhook, Telegram, GitHub and bridge channels' },
  { id: 'models', label: 'Models', section: 'Connect', icon: Boxes, path: '/models', digit: 5, hint: 'LLM endpoints: add, test, persist or revert' },
  { id: 'credentials', label: 'Credentials', section: 'Connect', icon: KeyRound, path: '/credentials', digit: 6, hint: 'Encrypted secrets, engine-wide or per user' },
  { id: 'safety', label: 'Safety', section: 'Govern', icon: ShieldCheck, path: '/safety', digit: 7, hint: 'Safety profiles and model-backed classifiers' },
  { id: 'users', label: 'Users', section: 'Govern', icon: Users, path: '/users', digit: 8, hint: 'Profiles, linked handles, quotas, model layer and invites' },
  { id: 'tenancy', label: 'Tenancy', section: 'Govern', icon: Building2, path: '/tenancy', hint: 'Groups, members, shared memory pools and grants' },
  { id: 'usage', label: 'Usage', section: 'Operate', icon: Activity, path: '/usage', hint: 'Token ledger per tenant, agent and model for a day' },
  { id: 'logs', label: 'Logs', section: 'Operate', icon: ScrollText, path: '/logs', digit: 9, hint: 'Live runtime log tail with filters' },
  { id: 'config', label: 'Config', section: 'Operate', icon: FileCog, path: '/config', hint: 'Raw config editor with boot-equivalent validation' },
  { id: 'docs', label: 'Documentation', section: 'Studio', icon: BookOpen, path: '/docs', hint: 'Bundled AgentFlow guides and reference' },
  { id: 'settings', label: 'Settings', section: 'Studio', icon: Settings, path: '/settings', hint: 'Appearance and connection' },
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
