// Deleting an agent, shared by the Agents list and the builder. The runtime
// answers 409 when the agent has live sessions (or was not created by
// Studio); that becomes a second, explicit "force" question.
import { ApiError } from '@/api/client'
import type { Client } from '@/api/client'
import { useSession } from '@/state/session'
import { confirm, toast, toastError } from '@/state/ui'

export async function deleteAgentFlow(client: Client, name: string): Promise<boolean> {
  const ok = await confirm({
    title: 'Delete agent',
    message: `Delete agent "${name}" and its persisted files?`,
    confirmLabel: 'Delete',
    danger: true,
  })
  if (!ok) return false
  try {
    await client.deleteAgent(name, true, false)
  } catch (err) {
    if (!(err instanceof ApiError) || err.status !== 409) {
      toastError(err)
      return false
    }
    const force = await confirm({
      title: 'Delete agent',
      message: `${err.message || `Agent "${name}" has live sessions. Stop them and delete it?`}\n\nForce the delete?`,
      confirmLabel: 'Force delete',
      danger: true,
    })
    if (!force) return false
    try {
      await client.deleteAgent(name, true, true)
    } catch (err2) {
      toastError(err2)
      return false
    }
  }
  toast('deleted ' + name, 'success')
  void useSession.getState().reloadCatalog()
  return true
}
