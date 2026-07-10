<!-- base_styles.vue — shared, global typography for every käsi page (docs/08).
     Included in each page's <head>; the <style> is deliberately NOT scoped, so it
     applies document-wide, under the per-page scoped styles that handle layout.
     A very basic, usable baseline: readable line height, a legible system font,
     clear headings, and distinguishable links — structure first, lightly dressed.

     The one script include (docs/16, decision-020): Turbo, but ONLY when a page
     passes a `turbo` src (the settings surface does; every other page omits it and
     a scoped missing-prop handler resolves it to "", so the v-if drops the tag).
     Turbo is a progressive enhancement — nothing depends on it (docs/08). -->
<template>
	<script v-if="turbo" :src="turbo" defer></script>
	<style>
		:root { color-scheme: light dark; --fg: #1b1b1b; --muted: #666; --link: #0b5aa2; --line: #d9d9d9; }
		@media (prefers-color-scheme: dark) {
			:root { --fg: #e6e6e6; --muted: #9a9a9a; --link: #6db3ff; --line: #333; }
		}

		*, *::before, *::after { box-sizing: border-box; }
		html { -webkit-text-size-adjust: 100%; }
		body {
			margin: 0;
			color: var(--fg);
			font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, Roboto, Helvetica, Arial, sans-serif;
			font-size: 16px;
			line-height: 1.6;
		}

		h1, h2, h3, h4 { line-height: 1.25; font-weight: 600; }
		h1 { margin: 0 0 0.5rem; }
		h2, h3 { margin: 1.5rem 0 0.5rem; }
		p { margin: 0.75rem 0; }
		ul, ol { margin: 0.75rem 0; }
		li { margin: 0.2rem 0; }

		a { color: var(--link); text-decoration: underline; text-underline-offset: 0.15em; }
		a:hover { text-decoration: none; }

		label { line-height: 1.4; }
		input, textarea, select, button { font: inherit; color: inherit; }
		button { cursor: pointer; }

		code, pre, textarea { font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace; }
		code { font-size: 0.9em; }
		pre {
			font-size: 0.85rem;
			line-height: 1.45;
			white-space: pre-wrap;
			overflow-wrap: anywhere;
			margin: 0.4rem 0;
		}

		hr { border: 0; border-top: 1px solid var(--line); margin: 1.5rem 0; }

		/* site_nav.vue — the ONE top-level navbar, on every page. Mobile-first:
		   the five links collapse behind a native <details>/<summary> tap toggle,
		   so the menu works with NO JavaScript (docs/08). At >=40rem the links
		   show inline as a horizontal bar and the toggle is hidden. The wide-screen
		   `.site-nav > .site-nav-links` rule out-specifies the UA rule that hides a
		   closed <details>'s content, so the bar is always visible there. */
		.site-nav { max-width: 40rem; margin: 1rem auto 0; padding: 0 1rem; }
		.site-nav-toggle {
			display: inline-block; cursor: pointer; list-style: none;
			font-size: 0.875rem; padding: 0.3rem 0.6rem;
			border: 1px solid var(--line); border-radius: 0.35rem; color: var(--fg);
		}
		.site-nav-toggle::-webkit-details-marker { display: none; }
		.site-nav-links {
			list-style: none; margin: 0.5rem 0 0; padding: 0;
			display: flex; flex-direction: column; gap: 0;
			border: 1px solid var(--line); border-radius: 0.35rem; overflow: hidden;
		}
		.site-nav-item { margin: 0; }
		.site-nav-link {
			display: block; padding: 0.55rem 0.75rem;
			text-decoration: none; color: var(--link);
		}
		.site-nav-link:hover { background: rgba(127, 127, 127, 0.12); text-decoration: none; }
		.site-nav-link.is-active { color: var(--fg); font-weight: 600; background: rgba(127, 127, 127, 0.14); }

		@media (min-width: 40rem) {
			.site-nav-toggle { display: none; }
			.site-nav > .site-nav-links {
				display: flex; flex-direction: row; flex-wrap: wrap; gap: 0.25rem;
				margin: 0; padding: 0.15rem 0; border: 0;
				border-bottom: 1px solid var(--line); border-radius: 0; overflow: visible;
			}
			.site-nav-link { padding: 0.35rem 0.6rem; border-radius: 0.35rem; }
			.site-nav-link.is-active { background: none; border-radius: 0; border-bottom: 2px solid currentColor; }
		}
	</style>
</template>
