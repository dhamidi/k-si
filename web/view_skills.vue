<!-- view_skills.vue — the skills registry (docs/08, Flow D): a scannable,
     single-column window into the skills agents have authored. Each row is a
     skill_row linking to its detail page. Empty state when nothing is authored
     yet. Mobile-first, semantic, structure before style (decision-005). -->
<template>
	<html lang="en">
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>käsi — skills</title>
		<base_styles></base_styles>
	</head>
	<body>
		<site_nav :nav="skills.Nav"></site_nav>
		<main class="view-skills">
			<h1>Skills</h1>

			<p v-if="skills.Skills.length == 0" class="empty">No skills yet.</p>

			<ul v-if="skills.Skills.length > 0">
				<skill_row v-for="row in skills.Skills" :skill="row"></skill_row>
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
