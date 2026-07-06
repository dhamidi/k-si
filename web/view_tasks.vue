<!-- view_tasks.vue — the task list (docs/08): a scannable, single-column window
     into the system. Tasks grouped by status, newest-first within each, empty
     groups omitted. Mobile-first, semantic, structure before style (decision-005).
     The component hierarchy mirrors the information hierarchy: page → status
     group → task_row. -->
<template>
	<html lang="en">
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>käsi — tasks</title>
	</head>
	<body>
		<main class="view-tasks">
			<h1>Tasks</h1>

			<p v-if="tasks.Groups.length == 0" class="empty">No tasks yet.</p>

			<section v-for="group in tasks.Groups" class="task-group">
				<h2>{{ group.Label }}</h2>
				<ul>
					<task_row v-for="row in group.Tasks" :task="row"></task_row>
				</ul>
			</section>
		</main>
	</body>
	</html>
</template>

<style scoped>
main { max-width: 40rem; margin: 2rem auto; padding: 0 1rem; font-family: system-ui, sans-serif; }
h1 { font-size: 1.5rem; margin: 0 0 1rem; }
h2 { font-size: 1rem; margin: 1.5rem 0 0.5rem; text-transform: uppercase; letter-spacing: 0.05em; }
ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 0.5rem; }
.empty { color: inherit; }
</style>
