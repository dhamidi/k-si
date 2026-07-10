<!-- view_transcript.vue — one agent run's session (docs/08, decision-007): user,
     assistant, thinking, tool calls and their results, and a status footer. The
     turns live inside a <turbo-frame id="run-transcript"> so a live run can update
     JUST that frame, not the whole page. The frame-wrapped turns are built once at
     the render edge (transcript_turns.vue rendered, then wrapped — htmlc cannot emit
     the hyphenated <turbo-frame> tag itself) and injected here via v-html, so the
     page and the frame fragment share the one turns template.

     Live refresh degrades in layers (docs/08 — no capability may depend on client
     scripting):
       * With NO JavaScript: while Active a <meta http-equiv="refresh"> reloads the
         WHOLE page every few seconds — the operator still watches the run progress,
         nothing lost (the original behaviour, kept as the fallback).
       * With JavaScript + Turbo: the inline script REMOVES that meta (so the page
         no longer whole-reloads) and instead reloads only the frame on an interval,
         preserving scroll and page chrome.
     A FINISHED run carries no meta, no script — a plain static frame. -->
<template>
	<html lang="en">
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<meta v-if="transcript.Active" id="transcript-fallback-refresh" http-equiv="refresh" content="5">
		<title>käsi — transcript</title>
		<base_styles :turbo="transcript.TurboSrc"></base_styles>
	</head>
	<body>
		<site_nav :nav="transcript.Nav"></site_nav>
		<main class="view-transcript">
			<header>
				<p><a :href="transcript.BackPath">← Task</a></p>
				<h1>Run {{ transcript.RunNumber }}</h1>
				<p v-if="transcript.Active" class="live">Live — updating as the agent works.</p>
			</header>

			<div class="frame-host" v-html="transcript.FrameHTML"></div>
		</main>

		<!-- Progressive enhancement (rendered only while Active): cancel the whole-page
		     meta fallback and reload just the frame on an interval. Turbo reloads the
		     frame from its own route (the current URL), which answers the Turbo-Frame
		     request with the bare frame fragment and swaps it in place. Absent JS this
		     script never runs and the meta fallback above drives a full-page reload. -->
		<script v-if="transcript.Active" id="transcript-refresh">
			(function () {
				var frame = document.getElementById('run-transcript');
				if (!frame) return;
				var meta = document.getElementById('transcript-fallback-refresh');
				if (meta && meta.parentNode) meta.parentNode.removeChild(meta);
				setInterval(function () {
					frame.setAttribute('src', window.location.href);
					if (typeof frame.reload === 'function') frame.reload();
				}, 5000);
			})();
		</script>
	</body>
	</html>
</template>

<style scoped>
main { max-width: 44rem; margin: 2rem auto; padding: 0 1rem; }
h1 { font-size: 1.5rem; margin: 0.25rem 0; }
.live { font-size: 0.875rem; }
</style>
