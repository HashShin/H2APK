# Web UI Gitlawb Redesign

**Date:** 2026-07-02
**Status:** approved

## Goal

Redesign the H2A web UI (`static/index.html`) to match the gitlawb design system aesthetic.

## Design System

### Theme
- Dark mode only. Background: `#0a0a0f`. Cards: `#12131a` with `rgba(255,255,255,0.08)` border.
- No pure black, no heavy drop shadows — glow borders instead.

### Palette
| Role | Color | Usage |
|------|-------|-------|
| Primary accent | `#7c5cff` | Links, buttons, focus, hero gradient |
| Status green | `#3ddc84` | Success indicators, pulsing dot |
| Heading text | `#f2f2f5` | Titles, labels |
| Body text | `#a0a0ad` | Paragraphs, secondary text |
| Muted text | `#6b6b78` | Meta, placeholders |
| Error | `#ff6b6b` | Error states |

### Typography
- **Inter** (Google Fonts) — body and headings, bold weight, tight letter-spacing on hero
- **JetBrains Mono** (Google Fonts) — textareas, URL input, log panel
- Hero: 56px desktop, gradient text
- Code/terminal: monospace with colored prompt

### Components
- **Cards:** 12px radius, border intensifies on hover (glow)
- **Button:** violet gradient fill, glow shadow on hover, 12px radius
- **Mode tabs:** rounded-full pill badges, violet accent on active
- **Log panel:** dark inset with window chrome dots (● ● ● red/yellow/green)
- **Status:** pulsing `#3ddc84` dot for done, violet spinner for building
- **Inputs:** dark bg, violet focus border

### Layout
- Max-width: 900px (scaled from 1100px reference for form-heavy tool)
- 120px padding above hero, 24px gap between sections

### Motion
- Sections fade-slide-in on page load
- Animated gradient glow drift on button
- Pulsing status dot (CSS animation)

## Implementation

Single file change: `static/index.html`. Rewrite the `<style>` block and minor HTML adjustments (log panel dots, Google Fonts link). No changes to `main.go` or JavaScript logic.

Scope: CSS-only redesign. HTML structure and JS remain unchanged except for window chrome dots in the log panel.
