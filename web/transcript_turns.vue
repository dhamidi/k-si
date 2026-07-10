<!-- transcript_turns.vue — the CONTENT of a run's transcript Turbo frame (docs/08,
     decision-007): the parsed turns, or an empty state. Rendered TWICE from this
     one template (exactly like setting_form.vue): embedded in the full page
     (view_transcript.vue, via v-html of the <turbo-frame>-wrapped output) and alone
     as the frame fragment Turbo swaps in place. The <turbo-frame> wrapper is applied
     at the render edge in Go — htmlc reads a hyphenated tag as a component reference
     and cannot emit <turbo-frame> itself (docs/16). Server-rendered either way, so
     the turns show with NO JavaScript; the live auto-refresh is layered on top. -->
<template>
	<div class="turns-body">
		<p v-if="transcript.Turns.length == 0" class="empty">No turns yet.</p>

		<ol class="turns">
			<transcript_turn v-for="turn in transcript.Turns" :turn="turn"></transcript_turn>
		</ol>
	</div>
</template>

<style scoped>
.turns { list-style: none; margin: 1rem 0 0; padding: 0; display: flex; flex-direction: column; gap: 1rem; }
.empty { font-size: 0.9375rem; }
</style>
