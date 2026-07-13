<!-- skill_row.vue — one skill in the registry list (docs/08, Flow D): name,
     description, origin, and a link to the detail page. Renders in isolation from
     a single SkillRow; the page loops it once per skill. Semantic, single-column. -->
<template>
	<li class="skill-row">
		<a :href="skill.ShowPath">
			<span class="name">{{ skill.Name }}</span>
			<span class="description">{{ skill.Description }}</span>
			<span class="meta">origin: {{ skill.Origin }}</span>
		</a>
		<form class="remove" method="post" :action="skill.DeletePath">
			<button type="submit">Remove</button>
		</form>
	</li>
</template>

<style scoped>
.skill-row { display: flex; align-items: flex-start; gap: 0.75rem; border-bottom: 1px solid; }
.skill-row a { display: flex; flex-direction: column; gap: 0.15rem; padding: 0.5rem 0; text-decoration: none; color: inherit; flex: 1; }
.name { font-weight: 600; }
.description { font-size: 0.9375rem; }
.meta { font-size: 0.875rem; }
.remove { margin: 0.5rem 0; }
</style>
