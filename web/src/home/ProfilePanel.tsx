import { Activity, Gauge, Mountain, Pause, Thermometer, TrendingUp } from 'lucide-react'
import { type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { useFormat } from './format'
import { ascent, movingHours, samples, summit, totalKm, type Sample } from './terrain'

const CH_W = 640
const CH_H = 220

export function ProfilePanel() {
  const { t } = useTranslation()
  const f = useFormat()
  const minE = 600
  const maxE = 3100
  const x = (s: Sample) => (s.km / totalKm) * CH_W
  const yE = (e: number) => CH_H - ((e - minE) / (maxE - minE)) * CH_H
  const yHr = (hr: number) => CH_H - ((hr - 80) / (190 - 80)) * CH_H
  const line = samples.map((s) => `${x(s).toFixed(1)} ${yE(s.ele).toFixed(1)}`).join('L')
  const area = `M0 ${CH_H}L${line}L${CH_W} ${CH_H}Z`
  const hr = `M${samples.map((s) => `${x(s).toFixed(1)} ${yHr(s.hr).toFixed(1)}`).join('L')}`

  // The steepest stretch of the ascent, as if someone had brushed it.
  const peak = samples.indexOf(summit)
  const from = Math.round(peak * 0.42)
  const range = samples.slice(from, peak + 1)
  const gain = ascent(range)
  const rangeKm = range[range.length - 1].km - range[0].km
  const bx0 = x(range[0])
  const bx1 = x(range[range.length - 1])
  const head = samples[Math.round(samples.length * 0.72)]

  return (
    <figure className="border-line bg-paper-2 rounded-[28px] border p-5 sm:p-7">
      <figcaption className="sr-only">{t('home.profileAlt')}</figcaption>
      <dl className="grid grid-cols-2 gap-x-6 gap-y-4 sm:grid-cols-4">
        <PanelStat icon={<Activity />} label={t('home.distance')} value={f.km(totalKm)} />
        <PanelStat icon={<TrendingUp />} label={t('home.ascent')} value={f.m(ascent(samples))} />
        <PanelStat icon={<Mountain />} label={t('home.highest')} value={f.m(summit.ele)} />
        <PanelStat icon={<Gauge />} label={t('home.movingTime')} value={f.duration(movingHours)} />
      </dl>
      <div className="mt-6 flex flex-wrap gap-2 text-xs font-medium">
        <SeriesChip color="var(--ele)" label={t('series.elevation')} on />
        <SeriesChip color="var(--hr)" label={t('series.heartRate')} on />
        <SeriesChip color="var(--speed)" label={t('series.speed')} />
        <SeriesChip color="var(--cad)" label={t('series.cadence')} />
        <SeriesChip color="var(--temp)" label={t('series.temperature')} icon={<Thermometer className="size-3" />} />
      </div>
      <div className="relative mt-4">
        <svg viewBox={`0 0 ${CH_W} ${CH_H}`} className="block h-auto w-full overflow-visible" aria-hidden>
          {[1000, 1500, 2000, 2500, 3000].map((e) => (
            <line key={e} x1={0} x2={CH_W} y1={yE(e)} y2={yE(e)} stroke="var(--line)" />
          ))}
          <rect x={bx0} y={0} width={bx1 - bx0} height={CH_H} fill="var(--ink)" opacity={0.06} />
          <line x1={bx0} x2={bx0} y1={0} y2={CH_H} stroke="var(--ink)" strokeOpacity={0.35} />
          <line x1={bx1} x2={bx1} y1={0} y2={CH_H} stroke="var(--ink)" strokeOpacity={0.35} />
          <path d={area} fill="var(--ele)" opacity={0.22} />
          <path d={`M${line}`} fill="none" stroke="var(--ele)" strokeWidth={2} />
          <path d={hr} fill="none" stroke="var(--hr)" strokeWidth={1.5} />
          <line x1={x(head)} x2={x(head)} y1={0} y2={CH_H} stroke="var(--trail)" strokeWidth={1.5} />
          <circle cx={x(head)} cy={yE(head.ele)} r={5} fill="var(--trail)" stroke="var(--paper-2)" strokeWidth={2} />
        </svg>
        <div
          className="bg-ink text-paper absolute -top-3 rounded-full px-3 py-1 text-xs font-medium whitespace-nowrap tabular-nums"
          style={{ left: `${((bx0 + bx1) / 2 / CH_W) * 100}%`, transform: 'translateX(-50%)' }}
        >
          {t('home.climb', { gain: f.m(gain), distance: f.km(rangeKm) })}
        </div>
      </div>
      <div className="mt-5 flex items-center gap-3 text-sm">
        <span className="bg-ink text-paper grid size-9 place-items-center rounded-full" aria-hidden>
          <Pause className="size-4 fill-current" />
        </span>
        <div className="bg-line relative h-1.5 flex-1 rounded-full">
          <div className="bg-trail absolute inset-y-0 left-0 rounded-full" style={{ width: '72%' }} />
        </div>
        <span className="text-ink-soft font-medium tabular-nums">60×</span>
      </div>
    </figure>
  )
}

function PanelStat({ icon, label, value }: { icon: ReactNode; label: string; value: string }) {
  return (
    <div>
      <dt className="text-ink-soft flex items-center gap-1.5 text-xs [&_svg]:size-3.5">
        {icon}
        {label}
      </dt>
      <dd className="font-display mt-1 text-xl font-semibold tracking-tight tabular-nums sm:text-2xl">{value}</dd>
    </div>
  )
}

function SeriesChip({ color, label, on, icon }: { color: string; label: string; on?: boolean; icon?: ReactNode }) {
  return (
    <span
      className={`flex items-center gap-1.5 rounded-full border px-2.5 py-1 ${on ? 'border-ink/20 bg-paper' : 'border-line text-ink-soft'}`}
    >
      {icon ?? <span className="size-2 rounded-full" style={{ background: color, opacity: on ? 1 : 0.5 }} />}
      {label}
    </span>
  )
}
