import type { ReactNode } from 'react'
import styles from './Panel.module.css'

export function Panel({ children, className }: { children: ReactNode; className?: string }) {
  return <section className={`${styles.panel} ${className ?? ''}`}>{children}</section>
}

export function PanelHeader({ title, actions }: { title: ReactNode; actions?: ReactNode }) {
  return (
    <header className={styles.head}>
      <span className={styles.title}>{title}</span>
      {actions && <span className={styles.actions}>{actions}</span>}
    </header>
  )
}
