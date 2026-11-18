// "On this page": the h2/h3 outline, with the section being read marked.
import { useEffect, useRef } from 'react'
import type { DocHeading } from '@/docs/library'
import { cx } from '@/components/ui'

export function Outline({ outline, active, onPick }: { outline: DocHeading[]; active: number; onPick: (index: number) => void }) {
  const list = useRef<HTMLDivElement>(null)

  // Long references outgrow the column; keep the marked entry in sight.
  useEffect(() => {
    list.current?.querySelector('[aria-current="true"]')?.scrollIntoView({ block: 'nearest' })
  }, [active])

  if (outline.length === 0) return null
  return (
    <aside className="hidden w-[232px] shrink-0 flex-col border-l border-line min-[1200px]:flex" aria-label="On this page">
      <div className="shrink-0 px-4 pb-1.5 pt-4 text-xs font-bold text-faint">On this page</div>
      <div ref={list} className="min-h-0 flex-1 overflow-y-auto pb-4 pl-4 pr-2">
        {outline.map((h, i) => (
          <button
            key={i}
            type="button"
            aria-current={i === active ? 'true' : undefined}
            onClick={() => onPick(i)}
            title={h.text}
            className={cx(
              'block w-full truncate border-l py-[3px] text-left text-xs leading-snug transition-colors duration-100',
              h.level > 2 ? 'pl-5' : 'pl-2.5',
              i === active ? 'border-accent font-medium text-accent' : 'border-line text-muted hover:border-faint hover:text-text',
            )}
          >
            {h.text}
          </button>
        ))}
      </div>
    </aside>
  )
}
