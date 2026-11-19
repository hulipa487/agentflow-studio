import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider, createHashRouter } from 'react-router-dom'
import './index.css'
import App from './App'
import { startDesktopBridge } from './lib/desktop'

// Hash routes: the build can be served as plain files from any path. A data
// router (rather than <HashRouter>) is what lets a screen with unsaved edits
// hold a navigation until the operator has answered.
const router = createHashRouter([{ path: '*', element: <App /> }])

// In the desktop app: the save dialog, window colour and first-run import.
startDesktopBridge()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <RouterProvider router={router} />
  </StrictMode>,
)
