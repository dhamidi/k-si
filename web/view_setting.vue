<!-- view_setting.vue — one setting's form page (docs/16, decision-020): the long
     help text, then the form, wrapped in a <turbo-frame id="setting-{key}"> so
     Turbo can swap it on a reshape. The frame-wrapped form is built once at the
     render edge (setting_form.vue rendered, then wrapped — htmlc cannot emit the
     hyphenated <turbo-frame> tag itself) and injected here via v-html, so the page
     and the reshape fragment share the one form component. base_styles pulls the
     one Turbo <script> because this page passes it a turbo src. Mobile-first,
     single column; works without JavaScript (the reshape degrades to a full-page
     POST). -->
<template>
	<html lang="en">
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>käsi — setting</title>
		<base_styles :turbo="setting.TurboSrc"></base_styles>
	</head>
	<body>
		<site_nav :nav="setting.Nav"></site_nav>
		<main class="view-setting">
			<h1>{{ setting.Short }}</h1>
			<p class="key"><code>{{ setting.Key }}</code></p>
			<p class="long">{{ setting.Long }}</p>

			<div class="frame-host" v-html="setting.FrameHTML"></div>
		</main>
	</body>
	</html>
</template>

<style scoped>
main { max-width: 32rem; margin: 2rem auto; padding: 0 1rem; }
h1 { font-size: 1.5rem; margin: 0 0 0.25rem; }
.key { margin: 0 0 0.75rem; font-size: 0.875rem; }
.long { font-size: 0.9375rem; margin: 0 0 1.5rem; }
</style>
