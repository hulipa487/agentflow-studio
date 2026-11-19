# AgentFlow brand assets

The AgentFlow logo and banners, copied from the shared `agentflow-assets`
kit so Studio carries the same branding as the runtime. This folder is the
reference copy; each app keeps the files it ships next to its own code:

| File | What it is | Used by |
| --- | --- | --- |
| `logo.svg` | The hexagon "A" mark on a transparent canvas | Desktop: in-app logo (`desktop/internal/ui/brand/`). Web: logo and favicon (`web/public/brand/`) |
| `logo.png` | The mark at 1024×1024 | Desktop: window and taskbar icon. Web: home-screen icon |
| `logo-banner-darkmode.svg` | Mark plus "AgentFlow" wordmark for dark backgrounds | Web: welcome screen in dark mode. README |
| `logo-banner-lightmode.svg` | The same for light backgrounds | Web: welcome screen in light mode. README |

The desktop app draws its banner from `logo.svg` and native text: Fyne's SVG
renderer does not draw SVG `<text>`, so the wordmark cannot come from the
banner files there.

When the kit changes, copy the new files here and into the two app folders.
