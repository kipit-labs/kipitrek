import { useTheme } from 'next-themes'
import { useTranslation } from 'react-i18next'

import { Segmented, SegmentedItem, Tip } from '@kipit-labs/kipitidy'

import { themes } from './themes'

/** Compact picker for menus; Settings shows the themes as option cards. */
export function ThemeSwitcher({ className }: { className?: string }) {
  const { t } = useTranslation()
  const { theme, setTheme } = useTheme()

  return (
    <Segmented
      size="sm"
      value={theme ?? 'system'}
      onValueChange={setTheme}
      aria-label={t('nav.theme')}
      className={className}
    >
      {themes.map(({ value, icon: Icon }) => (
        <Tip key={value} content={t(`theme.${value}`)}>
          <SegmentedItem value={value} aria-label={t(`theme.${value}`)}>
            <Icon />
          </SegmentedItem>
        </Tip>
      ))}
    </Segmented>
  )
}
