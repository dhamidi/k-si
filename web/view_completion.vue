<!-- view_completion.vue — the completion-link confirmation: this task is done (docs/08) -->
<template>
	<html lang="en">
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>käsi — task done</title>
	</head>
	<body>
		<main class="view-completion">
			<!-- lead with the decision; single column, semantic HTML, works
			     without JavaScript (docs/08) -->
			<article>
				<h1>Done ✓</h1>
				<p>This task has been marked complete. You can close this page.</p>
			</article>
		</main>
	</body>
	</html>
</template>

<style scoped>
main { max-width: 28rem; margin: 4rem auto; padding: 0 1rem; font-family: system-ui, sans-serif; }
h1 { font-size: 2rem; }
</style>
