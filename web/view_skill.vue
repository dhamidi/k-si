<!-- view_skill.vue — one skill's detail (docs/08, Flow D): the information
     hierarchy is metadata → file tree → SKILL.md body, and the layout matches it.
     Leads with what the skill is and where it came from (an agent-origin skill
     links to the task that authored it), then lists its tree (each entry a link
     to the raw file), then shows the SKILL.md body inline. Mobile-first, single
     column, structure before style (decision-005). -->
<template>
	<html lang="en">
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>käsi — skill</title>
		<base_styles></base_styles>
	</head>
	<body>
		<site_nav :nav="skill.Nav"></site_nav>
		<main class="view-skill">
			<header>
				<h1>{{ skill.Name }}</h1>
				<p class="description">{{ skill.Description }}</p>
				<ul class="meta">
					<li>origin: {{ skill.Origin }}</li>
					<li v-if="skill.HasOriginTask">from task
						<a :href="skill.OriginTaskPath">#{{ skill.OriginTask }}</a></li>
					<li>version: {{ skill.Version }}</li>
				</ul>
			</header>

			<section class="files">
				<h2>Files</h2>
				<ul>
					<li v-for="file in skill.Files">
						<a :href="file.FilePath">{{ file.Path }}</a>
					</li>
				</ul>
			</section>

			<section class="skill-md">
				<h2>SKILL.md</h2>
				<pre>{{ skill.SkillMD }}</pre>
			</section>
		</main>
	</body>
	</html>
</template>

<style scoped>
main { max-width: 40rem; margin: 2rem auto; padding: 0 1rem; }
h1 { font-size: 1.5rem; margin: 0.25rem 0; }
.description { margin: 0.25rem 0; }
.meta { font-size: 0.875rem; margin: 0.5rem 0; list-style: none; padding: 0; display: flex; flex-direction: row; flex-wrap: wrap; gap: 0.75rem; }
h2 { font-size: 1rem; margin: 1.5rem 0 0.5rem; text-transform: uppercase; letter-spacing: 0.05em; }
.files ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 0.25rem; }
</style>
