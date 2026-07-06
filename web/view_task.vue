<!-- view_task.vue — one task's detail (docs/08): the information hierarchy is
     status/subject/participants → runs → open request → artifacts, and the
     component tree matches it. Leads with what the task is and who is on it,
     then the agent runs (each a run_row, with a Stop form on the active run),
     then any open UI request, then the archived artifacts. Mobile-first,
     single column, structure before style (decision-005). -->
<template>
	<html lang="en">
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>käsi — task</title>
		<base_styles></base_styles>
	</head>
	<body>
		<main class="view-task">
			<!-- lead with status + subject + participants -->
			<header>
				<p class="status">{{ task.Status }}</p>
				<h1>{{ task.Subject }}</h1>
				<p class="route">{{ task.Route }}</p>
				<ul v-if="task.Participants.length > 0" class="participants">
					<li v-for="who in task.Participants">{{ who }}</li>
				</ul>
			</header>

			<!-- the agent runs -->
			<section class="runs">
				<h2>Runs</h2>
				<p v-if="task.Runs.length == 0" class="empty">No agent runs yet.</p>
				<ul v-if="task.Runs.length > 0">
					<run_row v-for="run in task.Runs" :run="run"></run_row>
				</ul>
			</section>

			<!-- any open UI request -->
			<section v-if="task.Request.Present" class="request">
				<h2>Open request</h2>
				<p>An agent is waiting on input.
					<a :href="task.Request.URL">Answer the request</a>.</p>
			</section>

			<!-- artifacts archived for this task -->
			<section v-if="task.Artifacts.length > 0" class="artifacts">
				<h2>Artifacts</h2>
				<ul>
					<li v-for="name in task.Artifacts">{{ name }}</li>
				</ul>
			</section>
		</main>
	</body>
	</html>
</template>

<style scoped>
main { max-width: 40rem; margin: 2rem auto; padding: 0 1rem; }
.status { font-size: 0.875rem; text-transform: uppercase; letter-spacing: 0.05em; margin: 0; }
h1 { font-size: 1.5rem; margin: 0.25rem 0; }
.route { font-size: 0.875rem; margin: 0.15rem 0; }
.participants { font-size: 0.875rem; margin: 0.15rem 0; list-style: none; padding: 0; display: flex; flex-direction: row; flex-wrap: wrap; gap: 0.5rem; }
h2 { font-size: 1rem; margin: 1.5rem 0 0.5rem; text-transform: uppercase; letter-spacing: 0.05em; }
ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 0.5rem; }
.artifacts ul { list-style: disc; padding-left: 1.25rem; }
</style>
