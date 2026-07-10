<!-- site_nav.vue — the ONE top-level navigation, shown on every käsi page (docs/08):
     the same five sections in the same order, the current one lit. A single
     component serves both layouts. On wide screens the links show inline as a
     horizontal bar and the <summary> toggle is hidden (CSS @media); on small
     screens the links collapse behind a tap-to-open <summary> — a native
     <details> disclosure, so the menu works with NO JavaScript (docs/08: no
     capability depends on client-side scripting). Built from a NavView the handler
     assembles (nav.go): each item's Path is reverse-routed and its Active flag is
     set in Go, never compared in the template. The styles live in base_styles.vue. -->
<template>
	<details class="site-nav">
		<summary class="site-nav-toggle">Menu</summary>
		<ul class="site-nav-links">
			<li v-for="item in nav.Items" class="site-nav-item">
				<a v-if="item.Active" class="site-nav-link is-active" aria-current="page" :href="item.Path">{{ item.Label }}</a>
				<a v-else class="site-nav-link" :href="item.Path">{{ item.Label }}</a>
			</li>
		</ul>
	</details>
</template>
