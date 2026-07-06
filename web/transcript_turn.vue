<!-- transcript_turn.vue — one turn of a run, rendered structurally by Kind
     (docs/08, decision-007): assistant text as prose, thinking dimmed/secondary,
     a tool call as its name + one-line input, a tool result as its output
     (flagged when it errored), and the final result as a footer. Renders in
     isolation from a single TurnView; the page loops it once per turn. Semantic,
     black-and-white, structure before style. -->
<template>
	<li class="turn">
		<article v-if="turn.Kind == 'assistant'" class="assistant">
			<h3>Assistant</h3>
			<pre>{{ turn.Text }}</pre>
		</article>

		<article v-if="turn.Kind == 'thinking'" class="thinking">
			<h3>Thinking</h3>
			<pre>{{ turn.Text }}</pre>
		</article>

		<article v-if="turn.Kind == 'user'" class="user">
			<h3>User</h3>
			<pre>{{ turn.Text }}</pre>
		</article>

		<article v-if="turn.Kind == 'tool_use'" class="tool-use">
			<h3>Tool: {{ turn.Tool }}</h3>
			<pre>{{ turn.Text }}</pre>
		</article>

		<article v-if="turn.Kind == 'tool_result'" class="tool-result">
			<h3>Result <span v-if="turn.IsError" class="error-flag">(error)</span></h3>
			<pre>{{ turn.Text }}</pre>
		</article>

		<footer v-if="turn.Kind == 'result'" class="footer">
			<strong v-if="turn.IsError">Run failed:</strong>
			<strong v-if="!turn.IsError">Run finished:</strong>
			{{ turn.Text }}
		</footer>
	</li>
</template>

<style scoped>
.turn { }
h3 { font-size: 0.75rem; text-transform: uppercase; letter-spacing: 0.05em; margin: 0 0 0.25rem; }
pre { margin: 0; white-space: pre-wrap; word-break: break-word; font: inherit; }
.thinking { opacity: 0.6; font-style: italic; }
.tool-use pre, .tool-result pre { font-family: ui-monospace, monospace; font-size: 0.875rem; }
.error-flag { font-weight: 600; }
.footer { border-top: 1px solid; padding-top: 0.5rem; font-size: 0.875rem; }
</style>
