import { useState } from 'react'
import { Banner, Button, Field, Input, Modal } from '@/components/ui'
import { t } from '@/i18n'
import { LUA_FILE_NAME_RE, PACKAGE_NAME_RE, normalizeLuaFilename } from './helpers'

export function NewPackageDialog({
  open,
  existingNames,
  onClose,
  onCreate,
}: {
  open: boolean
  existingNames: readonly string[]
  onClose: () => void
  onCreate: (name: string, file: string) => Promise<void>
}) {
  const [name, setName] = useState('')
  const [file, setFile] = useState('init.lua')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async () => {
    const pkg = name.trim()
    const luaFile = normalizeLuaFilename(file || 'init.lua')
    if (!PACKAGE_NAME_RE.test(pkg)) {
      setError(t('plugins.packageNameInvalid'))
      return
    }
    if (existingNames.includes(pkg)) {
      setError(t('plugins.packageExists', { name: pkg }))
      return
    }
    if (!LUA_FILE_NAME_RE.test(luaFile)) {
      setError(t('plugins.fileNameInvalid'))
      return
    }
    setBusy(true)
    setError('')
    try {
      await onCreate(pkg, luaFile)
      setName('')
      setFile('init.lua')
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      open={open}
      onClose={() => !busy && onClose()}
      title={t('plugins.dialogNewPackageTitle')}
      subtitle={t('plugins.dialogNewPackageSubtitle')}
      width={460}
      footer={
        <>
          <Button disabled={busy} onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" disabled={busy} onClick={() => void submit()}>
            {busy ? t('plugins.creating') : t('plugins.create')}
          </Button>
        </>
      }
    >
      <div className="space-y-3">
        {error && <Banner tone="danger">{error}</Banner>}
        <Field label={t('plugins.dialogPackageName')}>
          <Input
            mono
            autoFocus
            value={name}
            placeholder={t('plugins.dialogPackageNamePlaceholder')}
            onChange={(e) => {
              setName(e.target.value)
              setError('')
            }}
          />
        </Field>
        <Field label={t('plugins.dialogInitialFile')}>
          <Input
            mono
            value={file}
            placeholder={t('plugins.dialogFileNamePlaceholder')}
            onChange={(e) => {
              setFile(e.target.value)
              setError('')
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !busy) void submit()
            }}
          />
        </Field>
      </div>
    </Modal>
  )
}

export function NewFileDialog({
  open,
  pkgName,
  existingFiles,
  onClose,
  onCreate,
}: {
  open: boolean
  pkgName: string
  existingFiles: readonly string[]
  onClose: () => void
  onCreate: (file: string) => Promise<void>
}) {
  const [file, setFile] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async () => {
    const luaFile = normalizeLuaFilename(file)
    if (!LUA_FILE_NAME_RE.test(luaFile)) {
      setError(t('plugins.fileNameInvalid'))
      return
    }
    if (existingFiles.includes(luaFile)) {
      setError(t('plugins.fileExists', { file: luaFile }))
      return
    }
    setBusy(true)
    setError('')
    try {
      await onCreate(luaFile)
      setFile('')
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      open={open}
      onClose={() => !busy && onClose()}
      title={t('plugins.dialogNewFileTitle', { pkg: pkgName })}
      width={440}
      footer={
        <>
          <Button disabled={busy} onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" disabled={busy} onClick={() => void submit()}>
            {busy ? t('plugins.creating') : t('plugins.create')}
          </Button>
        </>
      }
    >
      <div className="space-y-3">
        {error && <Banner tone="danger">{error}</Banner>}
        <Field label={t('plugins.dialogFileName')}>
          <Input
            mono
            autoFocus
            value={file}
            placeholder={t('plugins.dialogFileNamePlaceholder')}
            onChange={(e) => {
              setFile(e.target.value)
              setError('')
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !busy) void submit()
            }}
          />
        </Field>
      </div>
    </Modal>
  )
}
