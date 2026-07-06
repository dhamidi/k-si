<!-- task_row.vue — one task in the list (docs/08): route, subject, status, and a
     link to the detail page. Renders in isolation from a single TaskRow; the
     page loops it once per task. Semantic and single-column. -->
<template>
	<li class="task-row">
		<a :href="task.ShowPath">
			<span class="subject">{{ task.Subject }}</span>
			<span class="meta">{{ task.Route }} · {{ task.Status }}</span>
		</a>
	</li>
</template>

<style scoped>
.task-row a { display: flex; flex-direction: column; gap: 0.15rem; padding: 0.5rem 0; text-decoration: none; color: inherit; border-bottom: 1px solid; }
.subject { font-weight: 600; }
.meta { font-size: 0.875rem; }
</style>
