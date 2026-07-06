<!-- view_counter.vue — the canary page: current count and the increment form (docs/08) -->
<template>
	<html lang="en">
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>käsi — counter</title>
		<base_styles></base_styles>
	</head>
	<body>
		<main class="view-counter">
			<h1>Counter</h1>
			<p>The count is <strong>{{ counter.Count }}</strong>.</p>

			<form method="post" :action="counter.IncrementPath">
				<label for="by">Increment by</label>
				<input id="by" name="by" inputmode="numeric" :value="counter.Form.By">
				<p class="error" v-if="counter.Form.Errors.by">{{ counter.Form.Errors.by }}</p>
				<button type="submit">Increment</button>
			</form>
		</main>
	</body>
	</html>
</template>

<style scoped>
main { max-width: 28rem; margin: 2rem auto; padding: 0 1rem; }
.error { color: #b00020; }
</style>
