<!-- view_settings.vue — the /settings index (docs/16, decision-020): every
     contributed setting with its short description, current value, and a link to
     its form. Mobile-first, single column, semantic; works without JavaScript —
     this page only reads (docs/08). -->
<template>
	<html lang="en">
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>käsi — settings</title>
		<base_styles></base_styles>
	</head>
	<body>
		<site_nav :nav="settings.Nav"></site_nav>
		<main class="view-settings">
			<h1>Settings</h1>
			<p class="lead">käsi's configuration. Edit any setting here; your change applies immediately.</p>

			<p v-if="settings.Spooling" class="warning" role="alert">Replies are being spooled to disk, not emailed. Set an Outbound sender to start delivering mail.</p>

			<p class="connections"><a :href="settings.CodexPath">Codex sign-in</a> — sign in to Codex so käsi can run Codex agents.</p>

			<p v-if="settings.Settings.length == 0" class="empty">No settings contributed.</p>

			<ul v-if="settings.Settings.length > 0">
				<li v-for="row in settings.Settings" class="setting-row">
					<div class="head">
						<a :href="row.ShowPath" class="name">{{ row.Short }}</a>
						<span class="owner">{{ row.Owner }}</span>
					</div>
					<p class="value"><code>{{ row.Key }}</code> = <span class="current">{{ row.Value }}</span></p>
				</li>
			</ul>
		</main>
	</body>
	</html>
</template>

<style scoped>
main { max-width: 40rem; margin: 2rem auto; padding: 0 1rem; }
h1 { font-size: 1.5rem; margin: 0 0 0.5rem; }
.lead { font-size: 0.875rem; margin: 0 0 1.5rem; }
.warning { font-size: 0.875rem; margin: 0 0 1.5rem; padding: 0.75rem; border: 1px solid #b00020; color: #b00020; border-radius: 0.25rem; }
.connections { font-size: 0.875rem; margin: 0 0 1.5rem; padding: 0 0 1rem; border-bottom: 1px solid; }
ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 0.75rem; }
.setting-row { border-bottom: 1px solid; padding: 0 0 0.75rem; }
.head { display: flex; align-items: baseline; justify-content: space-between; gap: 0.75rem; }
.name { font-weight: 600; }
.owner { font-size: 0.8125rem; }
.value { margin: 0.25rem 0 0; font-size: 0.875rem; }
.current { font-weight: 600; }
.empty { color: inherit; }
</style>
