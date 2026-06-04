// Curated Lucide line-icon set (spec D13: functional icons only, no emoji,
// consistent stroke/size). Re-export under stable Icon* names so call sites are
// decoupled from upstream icon renames. lucide-react icons are React components
// accepting { size?, strokeWidth?, className? }.
export {
  Search as IconSearch,
  Plus as IconPlus,
  SendHorizontal as IconSend,
  Square as IconStop,
  Settings as IconSettings,
  Sun as IconSun,
  Moon as IconMoon,
  Palette as IconPalette,
  ChevronRight as IconChevronRight,
  ChevronDown as IconChevronDown,
  X as IconX,
  File as IconFile,
  Folder as IconFolder,
  GitBranch as IconGitBranch,
  Command as IconCommand,
  TriangleAlert as IconAlertTriangle,
  Check as IconCheck,
  Copy as IconCopy,
  RefreshCw as IconRefresh,
  PanelLeft as IconPanelLeft,
  PanelRight as IconPanelRight,
  Activity as IconActivity,
  Coins as IconCoins,
  Database as IconDatabase,
} from 'lucide-react'

import type { LucideIcon } from 'lucide-react'
export type Icon = LucideIcon
