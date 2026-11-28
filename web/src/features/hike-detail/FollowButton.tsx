import { Footprints } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'

import { Button } from '@kipit-labs/kipitidy/button'
import { useRecord } from '@/features/record/store'

/** Opens the recorder on this planned route; a recording already under way is kept as is. */
export function FollowButton({ id }: { id: string }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  return (
    <Button
      variant="outline"
      size="sm"
      aria-label={t('record.followRoute')}
      title={t('record.followRoute')}
      onClick={() => {
        useRecord.getState().follow(id)
        navigate('/record')
      }}
    >
      <Footprints />
      {/* Icon only on phones, or the hike's name has no room left in the header. */}
      <span className="hidden sm:inline">{t('record.followRoute')}</span>
    </Button>
  )
}
