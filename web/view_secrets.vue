<!-- view_secrets.vue — the /secrets management surface (docs/06, decision-004):
     every stored secret as a REFERENCE with its last-set time (NEVER a value, no
     reveal control), an add/rotate form whose value field is masked and never
     echoes, a per-row delete that routes through a confirm step, and a compact
     name-only "Recent changes" audit trail. Mobile-first, semantic, single
     column, works with NO JavaScript (decision-005/006). A value NEVER appears on
     this page — the list and the trail deal in references alone. -->
<template>
	<html lang="en">
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>käsi — secrets</title>
		<base_styles></base_styles>
	</head>
	<body>
		<site_nav :nav="secrets.Nav"></site_nav>
		<main class="view-secrets">
			<h1>Secrets</h1>
			<p class="lead">Credentials käsi resolves inside effects. Stored by reference — the value is written once and never shown again. Setting an existing reference rotates it.</p>

			<form class="add" method="post" :action="secrets.SavePath">
				<h2>Add or rotate a secret</h2>
				<label for="namespace">Namespace</label>
				<input id="namespace" name="namespace" :value="secrets.Form.Namespace" placeholder="fastmail">
				<p class="error" v-if="secrets.Form.Errors.namespace">{{ secrets.Form.Errors.namespace }}</p>
				<label for="key">Key</label>
				<input id="key" name="key" :value="secrets.Form.Key" placeholder="api-token">
				<p class="error" v-if="secrets.Form.Errors.key">{{ secrets.Form.Errors.key }}</p>
				<label for="value">Value</label>
				<input id="value" name="value" type="password" autocomplete="off" placeholder="•••••••• (write-only, never shown again)">
				<button type="submit">Save</button>
			</form>

			<h2>Stored secrets</h2>
			<p v-if="secrets.Secrets.length == 0" class="empty">No secrets stored yet.</p>
			<ul v-if="secrets.Secrets.length > 0" class="secrets">
				<li v-for="row in secrets.Secrets" class="secret-row">
					<code class="ref">{{ row.Ref }}</code>
					<span class="updated">last set {{ row.UpdatedAt }}</span>
					<a class="delete" :href="row.DeletePath">Delete</a>
				</li>
			</ul>

			<h2>Recent changes</h2>
			<p v-if="secrets.Recent.length == 0" class="empty">No changes recorded yet.</p>
			<ul v-if="secrets.Recent.length > 0" class="audit">
				<li v-for="ev in secrets.Recent" class="audit-row">
					<span class="op">{{ ev.Op }}</span>
					<code class="ref">{{ ev.Ref }}</code>
					<span class="at">{{ ev.At }}</span>
				</li>
			</ul>
		</main>
	</body>
	</html>
</template>

<style scoped>
main { max-width: 40rem; margin: 2rem auto; padding: 0 1rem; }
h1 { font-size: 1.5rem; margin: 0 0 0.5rem; }
h2 { font-size: 1rem; margin: 1.5rem 0 0.5rem; }
.lead { font-size: 0.875rem; margin: 0 0 1.5rem; }
.add { display: flex; flex-direction: column; gap: 0.35rem; margin: 0 0 1rem; padding: 0 0 1.5rem; border-bottom: 1px solid; }
label { font-size: 0.875rem; font-weight: 600; }
input { font: inherit; padding: 0.35rem 0.5rem; }
button { align-self: flex-start; margin-top: 0.25rem; }
.error { color: #b00020; margin: 0; font-size: 0.875rem; }
ul { list-style: none; margin: 0.5rem 0; padding: 0; display: flex; flex-direction: column; gap: 0.5rem; }
.secret-row, .audit-row { display: flex; flex-wrap: wrap; align-items: baseline; gap: 0.5rem; }
.ref { font-size: 0.9rem; word-break: break-all; }
.updated, .at { color: var(--muted); font-size: 0.8rem; }
.op { font-size: 0.75rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.03em; }
.delete { margin-left: auto; font-size: 0.85rem; }
.empty { color: inherit; }
</style>
