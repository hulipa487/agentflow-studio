# AgentFlow brand assets

The AgentFlow logo and banners, copied from the shared `agentflow-assets`
kit so Studio carries the same branding as the runtime. This folder is the
reference copy; each app keeps the files it ships next to its own code:

| File | What it is | Used by |
| --- | --- | --- |
| `logo.svg` | The hexagon "A" mark on a transparent canvas | Web (and so the desktop app): logo and favicon (`web/public/brand/`) |
| `logo.png` | The mark at 1024×1024 | Desktop: window and taskbar icon, scaled to 16–256 px in `desktop/winres/`. Web: home-screen icon |
| `logo-banner-darkmode.svg` | Mark plus "AgentFlow" wordmark for dark backgrounds | Web: welcome screen in dark mode. README |
| `logo-banner-lightmode.svg` | The same for light backgrounds | Web: welcome screen in light mode. README |

When the kit changes, copy the new files here and into `web/public/brand/`, and
regenerate the desktop icons in `desktop/winres/` from `logo.png`.
