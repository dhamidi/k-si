<!-- view_confirm_delete_secret.vue — the no-JS safety step before a secret is
     deleted (docs/06, decision-004): "Delete <ref>? [Confirm] [Cancel]", so a
     critical credential (e.g. the Fastmail token käsi needs to send) is not
     fat-fingered away. Confirm POSTs the delete with the reference in a hidden
     field (a secret:// URL carries slashes and a scheme, so a hidden field beats
     encoding it in the path); Cancel is a plain link back. This page shows the
     REFERENCE only — never a value. Works with NO JavaScript (decision-005/006). -->
<template>
	<html lang="en">
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>käsi — delete secret</title>
		<base_styles></base_styles>
	</head>
	<body>
		<site_nav :nav="confirm.Nav"></site_nav>
		<main class="view-confirm-delete-secret">
			<h1>Delete secret</h1>
			<p class="prompt">Delete <code class="ref">{{ confirm.Ref }}</code>? This cannot be undone; any effect that resolves this reference will fail until it is set again.</p>
			<div class="actions">
				<form method="post" :action="confirm.DeletePath">
					<input type="hidden" name="ref" :value="confirm.Ref">
					<button type="submit" class="danger">Confirm delete</button>
				</form>
				<a class="cancel" :href="confirm.CancelPath">Cancel</a>
			</div>
		</main>
	</body>
	</html>
</template>

<style scoped>
main { max-width: 40rem; margin: 2rem auto; padding: 0 1rem; }
h1 { font-size: 1.5rem; margin: 0 0 0.5rem; }
.prompt { font-size: 0.95rem; }
.ref { word-break: break-all; }
.actions { display: flex; align-items: center; gap: 1rem; margin-top: 1rem; }
.danger { color: #b00020; }
.cancel { font-size: 0.9rem; }
</style>
