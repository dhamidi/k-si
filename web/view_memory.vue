<!-- view_memory.vue — the owner's memory curation page (docs/08, feature-memory.md):
     every remembered fact with its derived description and raw note, an add/edit
     form at the top, and a per-row edit + forget. Mobile-first, semantic, single
     column, works without JavaScript (decision-005). -->
<template>
	<html lang="en">
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>käsi — memory</title>
		<base_styles></base_styles>
	</head>
	<body>
		<main class="view-memory">
			<nav class="crumbs"><a :href="memory.TasksPath">Tasks</a></nav>
			<h1>Memory</h1>
			<p class="lead">Durable facts käsi carries into every task. Add, edit, or forget them here.</p>

			<form class="add" method="post" :action="memory.SavePath">
				<h2>Remember a fact</h2>
				<label for="name">Name</label>
				<input id="name" name="name" :value="memory.Form.Name" placeholder="reply-style">
				<p class="error" v-if="memory.Form.Errors.name">{{ memory.Form.Errors.name }}</p>
				<label for="content">Note</label>
				<textarea id="content" name="content" rows="8">{{ memory.Form.Content }}</textarea>
				<p class="error" v-if="memory.Form.Errors.content">{{ memory.Form.Errors.content }}</p>
				<button type="submit">Save</button>
			</form>

			<p v-if="memory.Memories.length == 0" class="empty">No memories yet.</p>

			<ul v-if="memory.Memories.length > 0">
				<li v-for="row in memory.Memories" class="memory-row">
					<h2>{{ row.Name }}</h2>
					<p class="description">{{ row.Description }}</p>
					<form class="edit" method="post" :action="memory.SavePath">
						<input type="hidden" name="name" :value="row.Name">
						<textarea name="content" rows="6">{{ row.Content }}</textarea>
						<div class="actions">
							<button type="submit">Save</button>
						</div>
					</form>
					<form class="forget" method="post" :action="row.ForgetPath">
						<button type="submit">Forget</button>
					</form>
				</li>
			</ul>
		</main>
	</body>
	</html>
</template>

<style scoped>
main { max-width: 40rem; margin: 2rem auto; padding: 0 1rem; }
.crumbs { font-size: 0.875rem; margin: 0 0 0.5rem; }
h1 { font-size: 1.5rem; margin: 0 0 0.5rem; }
h2 { font-size: 1rem; margin: 0 0 0.25rem; }
.lead { font-size: 0.875rem; margin: 0 0 1.5rem; }
.add { display: flex; flex-direction: column; gap: 0.35rem; margin: 0 0 2rem; padding: 0 0 1.5rem; border-bottom: 1px solid; }
label { font-size: 0.875rem; font-weight: 600; }
input, textarea { font: inherit; padding: 0.35rem 0.5rem; }
textarea { width: 100%; box-sizing: border-box; font-family: monospace; }
.error { color: #b00020; margin: 0; font-size: 0.875rem; }
ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 1.5rem; }
.memory-row { border-bottom: 1px solid; padding: 0 0 1.5rem; }
.description { font-size: 0.875rem; margin: 0 0 0.5rem; }
.edit { display: flex; flex-direction: column; gap: 0.35rem; }
.actions { display: flex; gap: 0.5rem; }
.forget { margin: 0.5rem 0 0; }
.empty { color: inherit; }
</style>
