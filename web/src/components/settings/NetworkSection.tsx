import { t } from '../../lib/i18n'
import { useConfig } from '../../lib/useConfig'
import { BrandedSelect } from '../BrandedSelect'
import { StateView } from '../StateViews'
import styles from './sections.module.css'

const MODES = ['auto', 'env', 'custom', 'off'] as const
const SCHEMES = ['http', 'https', 'socks5', 'socks5h'] as const

export function NetworkSection() {
  const { cfg, error, patch, clearError } = useConfig()

  if (!cfg) return <StateView kind="loading" message={t('settings.loading', 'Loading…')} />

  return (
    <div>
      <div className={styles.header}>
        <h2 className={styles.h2}>{t('settings.network', 'Network / Proxy')}</h2>
        <p className={styles.sub}>{t('settings.networkSub', 'Outbound proxy and network configuration.')}</p>
      </div>
      {error && (
        <div className={styles.inlineError} role="alert">
          <span>{error}</span>
          <button onClick={clearError} aria-label={t('settings.dismiss', 'Dismiss')}>×</button>
        </div>
      )}
      <div className={styles.group}>
        <label className={styles.field}>
          {t('settings.proxyMode', 'Proxy mode')}
          <BrandedSelect
            value={cfg.proxyMode}
            options={MODES.map((m) => ({ value: m, label: m }))}
            onChange={(v) => void patch({ proxyMode: v as (typeof MODES)[number] })}
            ariaLabel={t('settings.proxyMode', 'Proxy mode')}
            testid="network-mode"
          />
        </label>

        {cfg.proxyMode === 'custom' && (
          <>
            <label className={styles.field}>
              {t('settings.proxyScheme', 'Proxy scheme')}
              <BrandedSelect
                value={cfg.proxyScheme}
                options={SCHEMES.map((s) => ({ value: s, label: s }))}
                onChange={(v) => void patch({ proxyScheme: v as (typeof SCHEMES)[number] })}
                ariaLabel={t('settings.proxyScheme', 'Proxy scheme')}
                testid="network-scheme"
              />
            </label>
            <label className={styles.field}>
              {t('settings.proxyUrl', 'Proxy URL')}
              <input
                className={styles.input}
                type="text"
                value={cfg.proxyUrl}
                onChange={(e) => void patch({ proxyUrl: e.target.value })}
                placeholder="host:port"
              />
            </label>
            <label className={styles.field}>
              no_proxy
              <input
                className={styles.input}
                type="text"
                value={cfg.noProxy}
                onChange={(e) => void patch({ noProxy: e.target.value })}
                placeholder="localhost,127.0.0.1,.internal"
              />
            </label>
          </>
        )}
      </div>
      <p className={styles.note}>{t('settings.proxyEnvNote', 'In env mode, DEEPSEEKCODE_PROXY / HTTPS_PROXY / NO_PROXY from the environment are used.')}</p>
      <p className={styles.note}>{t('settings.cjkNote', 'Non-UTF-8 files (GBK / GB18030) are detected and round-tripped so CJK content is preserved on read and write.')}</p>
    </div>
  )
}
