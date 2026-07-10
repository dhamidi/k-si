<!-- view_apps.vue — the /apps page (docs/08, feature-apps.md): every
     registered app, its URL, käsi's registry status, its live state, and its
     recent logs. Mobile-first, single column, structure before style
     (decision-005); works without JavaScript — this page only reads, it never
     writes (docs/08). -->
<template>
	<html lang="en">
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>käsi — apps</title>
		<base_styles></base_styles>
	</head>
	<body>
		<site_nav :nav="apps.Nav"></site_nav>
		<main class="view-apps">
			<h1>Apps</h1>

			<p v-if="apps.Apps.length == 0" class="empty">No apps registered yet.</p>

			<ul v-if="apps.Apps.length > 0">
				<app_row v-for="row in apps.Apps" :app="row"></app_row>
			</ul>
		</main>
	</body>
	</html>
</template>

<style scoped>
main { max-width: 40rem; margin: 2rem auto; padding: 0 1rem; }
h1 { font-size: 1.5rem; margin: 0 0 1rem; }
ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 0.5rem; }
.empty { color: inherit; }
</style>
