import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { UpdateStatus } from '@/lib/desktop'

const desktopMock = vi.hoisted(() => ({
  getUpdateStatus: vi.fn<() => Promise<UpdateStatus>>(),
  checkForUpdates: vi.fn<() => Promise<UpdateStatus>>(),
  applyDesktopUpdate: vi.fn<() => Promise<{ ok: boolean; restarting?: boolean }>>(),
}))

vi.mock('@/lib/desktop', () => desktopMock)

import { useUI } from './ui'
import { startAutoUpdateLoop, stopAutoUpdateLoop, useUpdater } from './updater'

function statusOf(over: Partial<UpdateStatus> = {}): UpdateStatus {
  return {
    current_version: 'v0.1.0',
    latest_version: 'v0.1.0',
    update_available: false,
    is_dev: false,
    repo: 'hulipa487/agentflow-studio',
    platform: 'linux',
    arch: 'amd64',
    can_self_update: true,
    checked_at: '2026-11-25T00:00:00Z',
    ...over,
  }
}

describe('useUpdater auto-update state and background loop', () => {
  let store: Record<string, string>

  beforeEach(() => {
    vi.useFakeTimers()
    store = {}
    vi.stubGlobal('localStorage', {
      getItem: (k: string) => (k in store ? store[k] : null),
      setItem: (k: string, v: string) => {
        store[k] = String(v)
      },
      removeItem: (k: string) => {
        delete store[k]
      },
      clear: () => {
        store = {}
      },
    })
    desktopMock.getUpdateStatus.mockReset()
    desktopMock.checkForUpdates.mockReset()
    desktopMock.applyDesktopUpdate.mockReset()
    stopAutoUpdateLoop()
    useUI.setState({ toasts: [] })
    useUpdater.setState({
      status: null,
      checking: false,
      applying: false,
      error: '',
      autoCheck: true,
      autoInstall: false,
      autoAttemptedVersion: '',
      dismissedVersion: '',
    })
  })

  afterEach(() => {
    stopAutoUpdateLoop()
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('persists autoCheck and autoInstall and keeps them coupled cleanly', () => {
    desktopMock.checkForUpdates.mockResolvedValue(statusOf())

    expect(useUpdater.getState().autoCheck).toBe(true)
    expect(useUpdater.getState().autoInstall).toBe(false)

    useUpdater.getState().setAutoCheck(false)
    expect(store['agentflow-studio:auto-update']).toBe('0')

    // Enabling autoInstall also turns autoCheck back on and triggers an initial check when unchecked.
    useUpdater.getState().setAutoInstall(true)
    expect(useUpdater.getState().autoInstall).toBe(true)
    expect(useUpdater.getState().autoCheck).toBe(true)
    expect(store['agentflow-studio:auto-install']).toBe('1')
    expect(store['agentflow-studio:auto-update']).toBe('1')
    expect(desktopMock.checkForUpdates).toHaveBeenCalledTimes(1)

    // Turning autoCheck off also disables autoInstall.
    useUpdater.getState().setAutoCheck(false)
    expect(useUpdater.getState().autoCheck).toBe(false)
    expect(useUpdater.getState().autoInstall).toBe(false)
    expect(store['agentflow-studio:auto-install']).toBe('0')
  })

  it('auto-installs once per version on check() and stops retrying after a failure', async () => {
    const available = statusOf({ latest_version: 'v0.2.0', update_available: true })
    desktopMock.checkForUpdates.mockResolvedValue(available)
    desktopMock.applyDesktopUpdate.mockRejectedValue(new Error('network reset'))

    useUpdater.setState({ status: statusOf(), autoCheck: true, autoInstall: true })

    await useUpdater.getState().check(true)
    await vi.waitFor(() => expect(desktopMock.applyDesktopUpdate).toHaveBeenCalledTimes(1))
    expect(useUpdater.getState().autoAttemptedVersion).toBe('v0.2.0')
    expect(useUI.getState().toasts.at(-1)?.text).toContain('v0.2.0')

    // A second check for the same failed version must not retry auto-apply in a loop.
    await useUpdater.getState().check(true)
    expect(desktopMock.applyDesktopUpdate).toHaveBeenCalledTimes(1)
  })

  it('immediately applies when autoInstall is enabled while a self-updatable update is ready', async () => {
    desktopMock.applyDesktopUpdate.mockResolvedValue({ ok: true, restarting: true })
    useUpdater.setState({
      status: statusOf({ latest_version: 'v0.3.0', update_available: true }),
    })

    useUpdater.getState().setAutoInstall(true)
    await vi.waitFor(() => expect(desktopMock.applyDesktopUpdate).toHaveBeenCalledTimes(1))
    expect(useUpdater.getState().autoAttemptedVersion).toBe('v0.3.0')
  })

  it('does not auto-install dev builds or read-only installations', async () => {
    desktopMock.checkForUpdates.mockResolvedValue(
      statusOf({ is_dev: true, can_self_update: true, latest_version: 'v0.2.0', update_available: true }),
    )
    useUpdater.setState({ status: statusOf(), autoInstall: true })
    await useUpdater.getState().check(true)
    expect(desktopMock.applyDesktopUpdate).not.toHaveBeenCalled()

    desktopMock.checkForUpdates.mockResolvedValue(
      statusOf({ is_dev: false, can_self_update: false, managed_reason: 'readonly', latest_version: 'v0.2.0', update_available: true }),
    )
    await useUpdater.getState().check(true)
    expect(desktopMock.applyDesktopUpdate).not.toHaveBeenCalled()
  })

  it('runs background checks on interval only while autoCheck is enabled', async () => {
    desktopMock.checkForUpdates.mockResolvedValue(statusOf())
    startAutoUpdateLoop(1000)

    expect(desktopMock.checkForUpdates).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1000)
    expect(desktopMock.checkForUpdates).toHaveBeenCalledTimes(2)

    useUpdater.getState().setAutoCheck(false)
    await vi.advanceTimersByTimeAsync(2000)
    expect(desktopMock.checkForUpdates).toHaveBeenCalledTimes(2)

    useUpdater.getState().setAutoCheck(true)
    await vi.advanceTimersByTimeAsync(1000)
    expect(desktopMock.checkForUpdates).toHaveBeenCalledTimes(3)

    stopAutoUpdateLoop()
    await vi.advanceTimersByTimeAsync(2000)
    expect(desktopMock.checkForUpdates).toHaveBeenCalledTimes(3)
  })
})
