<!-- view_agents.vue — the Agents section (decision-024): the agents käsi can run to
     work on your tasks, each with its connection status, and the picker for which
     one new tasks use by default. Claude is built in; Codex signs in with a ChatGPT
     subscription (managed at /codex). Which state each agent is in is decided in Go
     (never compared here). Works with no JavaScript. Host-gated, no token
     (decision-006). -->
<template>
	<html lang="en">
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>käsi — Agents</title>
		<base_styles></base_styles>
	</head>
	<body>
		<site_nav :nav="agents.Nav"></site_nav>
		<main class="view-agents">
			<h1>Agents</h1>
			<p class="lead">The agents käsi can run to work on your tasks. Claude is built in; Codex uses your ChatGPT subscription.</p>

			<p v-if="agents.Warning" class="warning">{{ agents.Warning }}</p>

			<ul class="agents">
				<li v-for="agent in agents.Agents" class="agent">
					<div class="agent-head">
						<span class="name">{{ agent.Label }}</span>
						<span v-if="agent.Default" class="badge">Default</span>
					</div>
					<span class="status">{{ agent.Status }}</span>
					<a v-if="agent.ManagePath" class="manage" :href="agent.ManagePath">{{ agent.ManageLabel }}</a>
				</li>
			</ul>

			<section class="default">
				<h2>Default agent</h2>
				<p>The agent new tasks run on. A task already under way keeps the agent it started with.</p>
				<form method="post" :action="agents.SavePath">
					<label for="agent">Run new tasks on</label>
					<select id="agent" name="agent">
						<option v-for="opt in agents.Options" :value="opt.Value" :selected="opt.Selected">{{ opt.Label }}</option>
					</select>
					<button type="submit">Save</button>
				</form>
			</section>
		</main>
	</body>
	</html>
</template>

<style scoped>
main { max-width: 40rem; margin: 2rem auto; padding: 0 1rem; }
.lead { color: #555; }
.warning { padding: 0.6rem 0.8rem; border-left: 3px solid #b45309; background: #fff7ed; color: #7c2d12; }
ul.agents { list-style: none; margin: 1.5rem 0; padding: 0; display: flex; flex-direction: column; gap: 0.75rem; }
.agent { display: flex; flex-direction: column; gap: 0.15rem; padding: 0.75rem 1rem; border: 1px solid #e5e7eb; border-radius: 0.5rem; }
.agent-head { display: flex; align-items: center; gap: 0.5rem; }
.name { font-weight: 600; }
.badge { font-size: 0.75rem; padding: 0.05rem 0.4rem; border-radius: 0.25rem; background: #eef2ff; color: #3730a3; }
.status { color: #555; font-size: 0.9rem; }
.manage { font-size: 0.9rem; margin-top: 0.1rem; }
.default form { display: flex; align-items: flex-end; gap: 0.5rem; flex-wrap: wrap; }
.default label { display: flex; flex-direction: column; gap: 0.25rem; }
button { padding: 0.5rem 1rem; }
</style>
