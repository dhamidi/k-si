<!-- view_setting.vue — one setting's form (docs/16, decision-020): the long help
     text, then a control per form field (looped through field.vue), then a submit.
     A plain embedded full-page form — no turbo-frame, no reshape controls yet
     (deferred to phase 3). Mobile-first, single column; works without JavaScript
     (decision-005). -->
<template>
	<html lang="en">
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>käsi — setting</title>
		<base_styles></base_styles>
	</head>
	<body>
		<main class="view-setting">
			<nav class="crumbs"><a :href="setting.IndexPath">Settings</a></nav>
			<h1>{{ setting.Short }}</h1>
			<p class="key"><code>{{ setting.Key }}</code></p>
			<p class="long">{{ setting.Long }}</p>

			<form method="post" :action="setting.SavePath">
				<field v-for="fld in setting.Fields" :field="fld"></field>
				<button type="submit">Save</button>
			</form>
		</main>
	</body>
	</html>
</template>

<style scoped>
main { max-width: 32rem; margin: 2rem auto; padding: 0 1rem; }
.crumbs { font-size: 0.875rem; margin: 0 0 0.5rem; }
h1 { font-size: 1.5rem; margin: 0 0 0.25rem; }
.key { margin: 0 0 0.75rem; font-size: 0.875rem; }
.long { font-size: 0.9375rem; margin: 0 0 1.5rem; }
form { display: flex; flex-direction: column; gap: 1rem; }
button { align-self: flex-start; padding: 0.5rem 1rem; }
</style>
