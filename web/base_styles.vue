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
	</style>
</template>
