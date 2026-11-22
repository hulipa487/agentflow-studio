import { expect, test, type Page } from '@playwright/test'

const MOCK_URL = 'http://127.0.0.1:18089'
const VALID_TOKEN = 'test-admin-token'

async function connectToMock(page: Page) {
  await page.goto('/')
  await page.getByPlaceholder('http://127.0.0.1:8080').fill(MOCK_URL)
  await page.getByPlaceholder('ADMIN_TOKEN or an operator token').fill(VALID_TOKEN)
  await page.getByRole('button', { name: 'Connect' }).click()
  await expect(page).toHaveURL(/#\/overview$/)
  await expect(page.getByText('helper').first()).toBeVisible()
}

test.describe('AgentFlow Studio E2E (mock admin API)', () => {
  test('login rejects invalid token and connects via local proxy', async ({ page }) => {
    const errors: string[] = []
    page.on('pageerror', (err) => errors.push(err.message))

    await page.goto('/')
    await page.getByPlaceholder('http://127.0.0.1:8080').fill(MOCK_URL)
    await page.getByPlaceholder('ADMIN_TOKEN or an operator token').fill('wrong-token')
    await page.getByRole('button', { name: 'Connect' }).click()

    await expect(page.getByText(/Authentication failed|rejected/i).first()).toBeVisible()
    await expect(page).not.toHaveURL(/#\/overview$/)

    await page.getByPlaceholder('ADMIN_TOKEN or an operator token').fill(VALID_TOKEN)
    await page.getByRole('button', { name: 'Connect' }).click()

    await expect(page).toHaveURL(/#\/overview$/)
    await expect(page.getByText('helper').first()).toBeVisible()
    expect(errors).toEqual([])
  })

  test('builder renders Lua 5.1 editor without Luau drift', async ({ page }) => {
    const errors: string[] = []
    page.on('pageerror', (err) => errors.push(err.message))

    await connectToMock(page)
    await page.goto('/#/agents/helper')
    await expect(page.getByRole('heading', { name: 'helper' })).toBeVisible()

    await page.getByRole('tab', { name: 'Loop' }).click()
    await expect(page.getByRole('heading', { name: 'Loop (Lua 5.1)' })).toBeVisible()
    await expect(page.getByText('Lua 5.1 · inline diagnostics')).toBeVisible()
    await expect(page.locator('.cm-content')).toContainText('af.llm.chat')
    await expect(page.getByText(/Ln \d+, Col \d+\s+·\s+Lua\s+·/)).toBeVisible()
    expect(await page.locator('body').innerText()).not.toContain('Luau')
    expect(errors).toEqual([])
  })

  test('playground runs live chat, time-travel scrubber, replay eval, and side-by-side compare', async ({ page }) => {
    const errors: string[] = []
    page.on('pageerror', (err) => errors.push(err.message))

    await connectToMock(page)
    await page.goto('/#/playground?agent=helper')

    await page.getByLabel('Message the agent').fill('Hello mock agent')
    await page.getByRole('button', { name: 'Send' }).click()

    await expect(page.getByText('Echo (helper): Hello mock agent')).toBeVisible()
    await expect(page.getByText('Step 4/4')).toBeVisible()

    // Step backward and jump back to the end with time-travel controls.
    await page.getByRole('button', { name: 'Previous step' }).click()
    await expect(page.getByText('Step 3/4')).toBeVisible()
    await page.getByRole('button', { name: 'Jump to end' }).click()
    await expect(page.getByText('Step 4/4')).toBeVisible()

    // Pin live thread for comparison.
    await page.getByRole('button', { name: 'Pin run for comparison' }).click()

    // Run deterministic Replay & Eval scenario.
    await page.getByRole('tab', { name: 'Replay & Eval' }).click()
    await page.getByRole('button', { name: 'Run scenario' }).click()
    await expect(page.getByText('All assertions passed (1)')).toBeVisible()
    await expect(page.getByText('send_contains: Replay output')).toBeVisible()
    await expect(page.getByText('Replay output for helper')).toBeVisible()

    // Open Compare tab and verify side-by-side comparison.
    await page.getByRole('tab', { name: /Compare \(2\)/ }).click()
    await expect(page.getByText('Baseline (A)')).toBeVisible()
    await expect(page.getByText('Candidate (B)')).toBeVisible()
    await expect(page.getByText('Duration')).toBeVisible()
    await expect(page.getByText('Tokens')).toBeVisible()
    await expect(page.getByText('Replies')).toBeVisible()
    await expect(page.getByText('Execution diff')).toBeVisible()
    expect(errors).toEqual([])
  })

  test('settings displays connected endpoint and token storage status', async ({ page }) => {
    const errors: string[] = []
    page.on('pageerror', (err) => errors.push(err.message))

    await connectToMock(page)
    await page.goto('/#/settings')
    await expect(page.getByText(MOCK_URL).first()).toBeVisible()
    await expect(page.getByText('token in localStorage').first()).toBeVisible()
    expect(errors).toEqual([])
  })

  test('safety screen creates custom Lua filter, attaches it to profile, and switches to Simplified Chinese', async ({ page }) => {
    const errors: string[] = []
    page.on('pageerror', (err) => errors.push(err.message))

    await connectToMock(page)
    await page.goto('/#/safety')

    await expect(page.getByRole('heading', { name: 'Custom Lua filters' })).toBeVisible()
    await page.getByRole('button', { name: 'New Lua filter', exact: true }).click()

    await page.getByPlaceholder('pii_mask').fill('pii_mask')
    await page.getByRole('button', { name: 'Validate', exact: true }).click()
    await expect(page.getByText('Lua filter "pii_mask" compiled without errors.')).toBeVisible()

    await page.getByRole('button', { name: 'Save', exact: true }).click()
    await expect(page.getByText('pii_mask').first()).toBeVisible()

    // Attach lua:pii_mask to the default safety profile.
    await page.getByRole('button', { name: /\bdefault\b/i }).first().click()
    await page.getByRole('button', { name: 'lua:pii_mask' }).click()
    await page.getByRole('button', { name: 'Save', exact: true }).click()
    await expect(page.getByText('lua:pii_mask').first()).toBeVisible()

    // Verify i18n language switch renders localized Safety UI labels.
    await page.getByRole('radio', { name: '简体' }).click()
    await expect(page.getByRole('heading', { name: '自定义 Lua 过滤器' })).toBeVisible()
    await expect(page.getByText('入站, 出站', { exact: true })).toBeVisible()
    await expect(page.getByText('放行').first()).toBeVisible()
    expect(errors).toEqual([])
  })
})
