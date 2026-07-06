<!-- view_request.vue — the page an agent's UI request link opens (Flow C, decision-005).
     Spec-driven: one page renders any request from its form spec. Mobile-first,
     single column, semantic, works without JavaScript; structure before style. -->
<template>
	<html lang="en">
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>käsi — request</title>
		<base_styles></base_styles>
	</head>
	<body>
		<main class="view-request">
			<!-- lead with the ask (the information hierarchy: request → fields) -->
			<request_summary :message="request.Message"></request_summary>

			<!-- closed state once answered; the link stops accepting input -->
			<request_answered v-if="request.Answered"></request_answered>

			<!-- otherwise the single-column form, one control per spec field.
			     multipart so file uploads work; no client scripting required. -->
			<form v-if="!request.Answered" method="post" enctype="multipart/form-data" :action="request.Action">
				<request_field v-for="field in request.Fields" :field="field"></request_field>
				<button type="submit">Submit</button>
			</form>
		</main>
	</body>
	</html>
</template>

<style scoped>
main { max-width: 32rem; margin: 2rem auto; padding: 0 1rem; }
form { display: flex; flex-direction: column; gap: 1rem; margin-top: 1.5rem; }
button { align-self: flex-start; padding: 0.5rem 1rem; }
</style>
