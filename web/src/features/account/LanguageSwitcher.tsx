import { useTranslation } from 'react-i18next'

import { Segmented, SegmentedItem, Tip } from '@kipit-labs/kipitidy'
import { isLanguage } from '@/i18n'
import { languageOptions } from '@/i18n/languages'

import { useSetLanguage } from './useLanguage'

/** Compact flag picker for menus and headers; Settings shows the languages as option cards. */
export function LanguageSwitcher({ className }: { className?: string }) {
  const { t, i18n } = useTranslation()
  const setLanguage = useSetLanguage()

  return (
    <Segmented
      size="sm"
      value={i18n.language}
      onValueChange={(v) => isLanguage(v) && setLanguage(v)}
      aria-label={t('settings.language')}
      className={className}
    >
      {languageOptions.map(({ value, label, flag: Flag }) => (
        <Tip key={value} content={<span lang={value}>{label}</span>}>
          <SegmentedItem value={value} aria-label={label} lang={value}>
            <Flag className="h-3 w-4.5 rounded-[2px] shadow-[0_0_0_0.5px_rgb(0_0_0/0.2)]" />
          </SegmentedItem>
        </Tip>
      ))}
    </Segmented>
  )
}
