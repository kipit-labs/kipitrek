import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

/** Number formats in the page's language. */
export function useFormat() {
  const { i18n } = useTranslation()
  return useMemo(() => {
    const whole = new Intl.NumberFormat(i18n.language, { maximumFractionDigits: 0 })
    const tenth = new Intl.NumberFormat(i18n.language, { minimumFractionDigits: 1, maximumFractionDigits: 1 })
    return {
      whole: (n: number) => whole.format(n),
      km: (n: number) => `${tenth.format(n)} km`,
      m: (n: number) => `${whole.format(n)} m`,
      duration: (h: number) => `${Math.floor(h)} h ${String(Math.round((h % 1) * 60)).padStart(2, '0')}`,
    }
  }, [i18n.language])
}
