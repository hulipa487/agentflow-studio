// Logs: the runtime's live log tail. The ring is replayed on connect, then
// lines arrive as they are written; the stream reconnects by itself.
import { useCallback, useState } from 'react'
import { Copy, Eraser, Pause, Play } from 'lucide-react'
import { featureOff } from '@/api/client'
import { Button, Dot, EmptyState, IconButton, Page, Panel, SearchBox, Select } from '@/components/ui'
import { t } from '@/i18n'
import { useScreenActions } from '@/lib/hooks'
import { copyText } from '@/lib/format'
import { useClient } from '@/state/session'
import { toast } from '@/state/ui'
import { DEFAULT_FLOOR, LEVEL_FLOORS } from './logs/buffer'
import { LogList } from './logs/LogList'
import { useLogTail } from './logs/useLogTail'

export default function Logs() {
  const client = useClient()
  const { buffer, version, status, error, refresh, restart } = useLogTail(client)
  const [filter, setFilter] = useState('')
  const [floor, setFloor] = useState(DEFAULT_FLOOR)
  useScreenActions({ refresh: restart })

  const copyLine = useCallback(async (text: string) => {
    if (await copyText(text)) toast(t('logs.lineCopied'), 'success')
    else toast(t('common.copyFailed'), 'danger')
  }, [])

  const copyVisible = async () => {
    const lines = buffer.visible
    if (await copyText(lines.map((l) => l.text).join('\n'))) toast(t('logs.linesCopied', { n: lines.length }), 'success')
    else toast(t('common.copyFailed'), 'danger')
  }

  const togglePause = () => {
    if (buffer.paused) buffer.resume()
    else buffer.pause()
    refresh()
  }

  const shown = buffer.visible.length
  const off = buffer.total === 0 && shown === 0 ? featureOff(error) : null
  const statusText = t(status.key, status.params)

  return (
    <Page
      title={t('logs.title')}
      subtitle={t('logs.subtitle')}
      scroll={false}
      meta={
        <>
          <span className="inline-flex min-w-0 items-center gap-1.5">
            <Dot tone={status.tone} pulse={!status.live} className="!size-1.5" />
            <span className="truncate">{statusText}</span>
          </span>
          {/* A paused view is a snapshot; the buffer's running total says nothing about it. */}
          <span>{buffer.paused || shown === buffer.total ? t('logs.lines', { n: shown }) : t('logs.linesShown', { shown, total: buffer.total })}</span>
          {buffer.paused && <span className="text-warning">{t('logs.paused')}</span>}
        </>
      }
    >
      <div className="mb-2 flex shrink-0 items-center gap-2">
        <SearchBox
          value={filter}
          onChange={(q) => {
            setFilter(q)
            buffer.setFilter(q)
            refresh()
          }}
          placeholder={t('logs.filterPlaceholder')}
          className="min-w-0 flex-1"
        />
        <Select
          aria-label={t('logs.levelFloor')}
          value={floor}
          onChange={(v) => {
            setFloor(v)
            buffer.setMinLevel(Number(v))
            refresh()
          }}
          options={LEVEL_FLOORS.map((f) => ({ value: f.value, label: t(f.labelKey) }))}
          className="w-[190px] shrink-0"
        />
        <Button icon={buffer.paused ? Play : Pause} onClick={togglePause} className="min-w-[92px] tabular-nums">
          {buffer.paused ? (buffer.pending > 0 ? t('logs.resumePending', { n: buffer.pending }) : t('logs.resume')) : t('logs.pause')}
        </Button>
        <Button
          icon={Eraser}
          onClick={() => {
            buffer.clear()
            refresh()
          }}
        >
          {t('common.clear')}
        </Button>
        <IconButton icon={Copy} label={t('logs.copyVisible')} onClick={copyVisible} disabled={shown === 0} />
      </div>
      {off ? (
        <EmptyState title={t('common.notEnabled')} guidance={off} />
      ) : (
        <Panel flush className="flex min-h-0 flex-1 flex-col overflow-hidden py-1">
          <LogList
            buffer={buffer}
            version={version}
            onCopy={copyLine}
            empty={buffer.total === 0 ? (status.live ? t('logs.emptyLive') : statusText) : t('logs.noMatch')}
          />
        </Panel>
      )}
    </Page>
  )
}
