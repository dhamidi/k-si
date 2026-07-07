<!-- task_row.vue — one task in the list (docs/08): route, subject, status, a link
     to the detail page, and a host-gated "Done" action (decision-006). Renders in
     isolation from a single TaskRow; the page loops it once per task. Semantic,
     single-column. The Done form is a sibling of the link, never nested in it. -->
<template>
	<li class="task-row">
		<a :href="task.ShowPath" class="open">
			<span class="subject">{{ task.Subject }}</span>
			<span class="meta">{{ task.Route }} · {{ task.Status }}</span>
		</a>
		<form v-if="task.MarkDonePath" method="post" :action="task.MarkDonePath" class="done">
			<button type="submit">Done</button>
		</form>
	</li>
</template>

<style scoped>
.task-row { display: flex; align-items: center; gap: 0.75rem; border-bottom: 1px solid; }
.open { flex: 1; display: flex; flex-direction: column; gap: 0.15rem; padding: 0.5rem 0; text-decoration: none; color: inherit; }
.subject { font-weight: 600; }
.meta { font-size: 0.875rem; }
.done { margin: 0; }
.done button { padding: 0.25rem 0.6rem; }
</style>
