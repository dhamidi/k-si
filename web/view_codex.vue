<!-- view_codex.vue — the Codex sign-in surface (decision-025): käsi signs in to
     Codex once, on the operator's behalf, and holds the result as a stored
     credential the Codex agent runs on. The page shows exactly one of four states
     (decided in Go, never compared here) and works with NO JavaScript: while a
     sign-in is under way a meta-refresh re-checks it on its own, and a "Check now"
     link does the same by hand. The one-time code and URL are PUBLIC — the operator
     types them into their own browser; the credential itself never reaches this
     page (decision-004). Host-gated, no token (decision-006). -->
<template>
	<html lang="en">
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<meta v-if="codex.Waiting" http-equiv="refresh" :content="codex.Refresh">
		<title>käsi — Codex sign-in</title>
		<base_styles></base_styles>
	</head>
	<body>
		<site_nav :nav="codex.Nav"></site_nav>
		<main class="view-codex">
			<h1>Codex sign-in</h1>
			<p class="lead">Sign in to Codex once here so käsi can run Codex agents on your tasks. käsi keeps the sign-in and uses it each time it starts one.</p>

			<section v-if="codex.Disconnected" class="state disconnected">
				<h2>You are not signed in</h2>
				<p>käsi cannot run Codex agents until you sign in. Signing in opens a code you enter on OpenAI's site — you approve it in your own browser, and nothing is sent back to this machine.</p>
				<form method="post" :action="codex.ConnectPath">
					<button type="submit">Sign in to Codex</button>
				</form>
			</section>

			<section v-if="codex.Waiting" class="state waiting">
				<h2>Finish signing in</h2>
				<p>Open the sign-in page and enter this code. The code expires in about 15 minutes.</p>
				<p class="code-line">
					<a class="url" :href="codex.VerificationURL">{{ codex.VerificationURL }}</a>
					<code class="code">{{ codex.Code }}</code>
				</p>
				<p class="hint">This page checks on its own once you approve. <a :href="codex.PollPath">Check now</a>.</p>
				<form method="post" :action="codex.DisconnectPath">
					<button type="submit" class="secondary">Cancel</button>
				</form>
			</section>

			<section v-if="codex.Connected" class="state connected">
				<h2>You are signed in</h2>
				<p>käsi is signed in to Codex and will use this sign-in when it runs Codex agents. The sign-in is kept with your other <a :href="codex.SecretsPath">secrets</a>, where you can review or remove it.</p>
				<form method="post" :action="codex.DisconnectPath">
					<button type="submit" class="secondary">Sign out</button>
				</form>
			</section>

			<section v-if="codex.Expired" class="state expired">
				<h2>The sign-in did not finish</h2>
				<p>The code expired or the sign-in was declined. Start again to get a fresh code.</p>
				<form method="post" :action="codex.ConnectPath">
					<button type="submit">Try again</button>
				</form>
			</section>
		</main>
	</body>
	</html>
</template>

<style scoped>
main { max-width: 40rem; margin: 2rem auto; padding: 0 1rem; }
h1 { font-size: 1.5rem; margin: 0 0 0.5rem; }
h2 { font-size: 1.125rem; margin: 0 0 0.5rem; }
.lead { font-size: 0.875rem; margin: 0 0 1.5rem; }
.state { border-top: 1px solid; padding: 1rem 0 0; }
.state p { font-size: 0.9375rem; }
.code-line { display: flex; flex-wrap: wrap; align-items: baseline; gap: 0.75rem; margin: 1rem 0; }
.url { font-size: 0.9375rem; word-break: break-all; }
.code { font-size: 1.5rem; font-weight: 700; letter-spacing: 0.08em; }
.hint { font-size: 0.8125rem; }
button { font: inherit; padding: 0.4rem 0.9rem; }
button.secondary { font-weight: 400; }
</style>
