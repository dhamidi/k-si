<!-- view_skill_file.vue — one entry of a skill's tree (docs/08, Flow D): the
     raw text of a single file in a <pre>, the leaf that makes the tree browsable.
     Leads with the file's path under its skill and a link back to the detail.
     Mobile-first, single column, structure before style (decision-005). -->
<template>
	<html lang="en">
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>käsi — skill file</title>
		<base_styles></base_styles>
	</head>
	<body>
		<main class="view-skill-file">
			<nav class="crumbs"><a :href="file.BackPath">{{ file.SkillName }}</a></nav>
			<h1>{{ file.Path }}</h1>
			<pre>{{ file.Content }}</pre>
		</main>
	</body>
	</html>
</template>

<style scoped>
main { max-width: 40rem; margin: 2rem auto; padding: 0 1rem; }
.crumbs { font-size: 0.875rem; margin: 0 0 0.5rem; }
h1 { font-size: 1.125rem; margin: 0.25rem 0 1rem; font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
</style>
