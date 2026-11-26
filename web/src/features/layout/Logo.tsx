import { cn } from '@/lib/utils'

/** The kipit-labs wordmark: the mark and the name, in the display font. */
export function Logo({ className }: { className?: string }) {
  return (
    <span className={cn('font-display inline-flex items-center gap-2 font-bold tracking-[-0.02em]', className)}>
      <LogoMark className="size-[1.15em]" />
      kipitrek
    </span>
  )
}

/** kipitiny's outlined square, with a kipit-orange peak where it has its yellow block. Same drawing as favicon.svg. */
export function LogoMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 26 26" aria-hidden="true" className={cn('shrink-0', className)}>
      <rect x="1" y="1" width="24" height="24" rx="5" fill="none" stroke="currentColor" strokeWidth="2" />
      <path d="M5.5 20 9.5 13.5 13.5 20Z" fill="#ff6b4a" stroke="#ff6b4a" strokeWidth="1" strokeLinejoin="round" />
    </svg>
  )
}
