<!-- run_row.vue — one agent run in the task detail (docs/08): its number, a link
     to the transcript, and — only for the active run — a Stop form. Renders in
     isolation from a single RunRow; the page loops it once per run. The Stop is
     a plain HTML POST (docs/08, "Stopping an agent"): one tap emits
     stop-agent-run; no JavaScript required. -->
<template>
	<li class="run-row">
		<span class="number">Run {{ run.Number }}</span>
		<a :href="run.TranscriptPath">Transcript</a>
		<form v-if="run.Active" method="post" :action="run.StopPath" class="stop">
			<button type="submit">Stop</button>
		</form>
	</li>
</template>

<style scoped>
.run-row { display: flex; align-items: center; gap: 0.75rem; padding: 0.5rem 0; border-bottom: 1px solid; }
.number { font-weight: 600; }
.stop { margin: 0; }
button { padding: 0.25rem 0.75rem; }
</style>
