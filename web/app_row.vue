<!-- app_row.vue — one registered app in the /apps list (docs/08,
     feature-apps.md): its URL, käsi's registry status, its live state on the
     machine, and its most recent journald lines. Renders in isolation from a
     single AppRow; the page loops it once per app. Semantic, single-column. -->
<template>
	<li class="app-row">
		<div class="head">
			<span class="name">{{ app.Name }}</span>
			<span class="right"><span class="meta">{{ app.Status }} · {{ app.Live }}</span><a v-if="app.URL" :href="app.URL" class="open" target="_blank" rel="noopener noreferrer">Open ↗</a></span>
		</div>
		<p v-if="app.Logs.length == 0" class="empty">No recent logs.</p>
		<ul v-if="app.Logs.length > 0" class="logs">
			<li v-for="line in app.Logs">{{ line }}</li>
		</ul>
	</li>
</template>

<style scoped>
.app-row { border-bottom: 1px solid; padding: 0.5rem 0; }
.head { display: flex; align-items: baseline; justify-content: space-between; gap: 0.75rem; }
.name { font-weight: 600; }
.right { display: flex; align-items: baseline; gap: 0.75rem; }
.meta { font-size: 0.875rem; }
.open { font-size: 0.875rem; font-weight: 600; text-decoration: none; white-space: nowrap; padding: 0.15rem 0.6rem; border: 1px solid currentColor; border-radius: 0.35rem; color: inherit; }
.open:hover { background: rgba(127, 127, 127, 0.15); }
.logs { list-style: none; margin: 0.5rem 0 0; padding: 0.5rem; background: rgba(127, 127, 127, 0.1); font-family: monospace; font-size: 0.8125rem; display: flex; flex-direction: column; gap: 0.15rem; overflow-x: auto; }
.empty { font-size: 0.875rem; margin: 0.35rem 0 0; }
</style>
