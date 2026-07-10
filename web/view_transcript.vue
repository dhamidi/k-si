<!-- view_transcript.vue — one agent run's session, rendered legibly (docs/08,
     decision-007): user turns, assistant turns, tool calls and their results,
     and a status footer. Each turn is a transcript_turn, rendered structurally
     by Kind. While the run is active the page self-refreshes via a meta refresh
     so new turns appear; with no JavaScript this is just a periodic reload —
     nothing is lost (docs/08). Structure before style. -->
<template>
	<html lang="en">
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<meta v-if="transcript.Active" http-equiv="refresh" content="5">
		<title>käsi — transcript</title>
		<base_styles></base_styles>
	</head>
	<body>
		<site_nav :nav="transcript.Nav"></site_nav>
		<main class="view-transcript">
			<header>
				<p><a :href="transcript.BackPath">← Task</a></p>
				<h1>Run {{ transcript.RunNumber }}</h1>
				<p v-if="transcript.Active" class="live">Live — updating as the agent works.</p>
			</header>

			<p v-if="transcript.Turns.length == 0" class="empty">No turns yet.</p>

			<ol class="turns">
				<transcript_turn v-for="turn in transcript.Turns" :turn="turn"></transcript_turn>
			</ol>
		</main>
	</body>
	</html>
</template>

<style scoped>
main { max-width: 44rem; margin: 2rem auto; padding: 0 1rem; }
h1 { font-size: 1.5rem; margin: 0.25rem 0; }
.live { font-size: 0.875rem; }
.turns { list-style: none; margin: 1rem 0 0; padding: 0; display: flex; flex-direction: column; gap: 1rem; }
</style>
