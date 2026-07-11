<!-- view_store.vue — the agent's persistent store, browsed as a directory tree
     (docs/08, Flow F decision-012): the operator's window into store/ — apps,
     memory, and scratch. A page is one of two shapes: a directory listing (dirs
     first, then files, each a link deeper) or a single file (text inline, or a
     download link for a large/binary file). Leads with the breadcrumb trail back
     up the tree. Mobile-first, single column, structure before style. Works with
     NO JavaScript — this page only reads, it never writes (docs/08). -->
<template>
	<html lang="en">
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>käsi — store</title>
		<base_styles></base_styles>
	</head>
	<body>
		<site_nav :nav="store.Nav"></site_nav>
		<main class="view-store">
			<nav class="crumbs">
				<span v-for="crumb in store.Crumbs" class="crumb"><a :href="crumb.Path">{{ crumb.Name }}</a> / </span>
			</nav>

			<div v-if="store.IsFile">
				<h1 class="filename">{{ store.Path }}</h1>
				<p class="filemeta">{{ store.File.SizeText }} · <a :href="store.File.RawPath">Download</a></p>
				<pre v-if="store.File.IsText">{{ store.File.Text }}</pre>
				<p v-if="store.File.TooLarge" class="notice">File too large to display ({{ store.File.SizeText }}). <a :href="store.File.RawPath">Download</a>.</p>
				<p v-if="store.File.IsBinary" class="notice">Binary file ({{ store.File.SizeText }}). <a :href="store.File.RawPath">Download</a>.</p>
			</div>

			<div v-else>
				<h1>Store</h1>
				<p v-if="store.Empty" class="empty">Nothing here yet.</p>
				<ul v-else class="entries">
					<li v-for="entry in store.Entries" class="entry">
						<a :href="entry.Path">
							<span class="name">{{ entry.Name }}</span>
							<span v-if="entry.IsDir" class="kind">dir</span>
							<span v-else class="size">{{ entry.SizeText }}</span>
						</a>
					</li>
				</ul>
			</div>
		</main>
	</body>
	</html>
</template>

<style scoped>
main { max-width: 40rem; margin: 2rem auto; padding: 0 1rem; }
.crumbs { font-size: 0.875rem; margin: 0 0 0.5rem; }
.crumb a { text-decoration: none; }
h1 { font-size: 1.5rem; margin: 0.25rem 0 1rem; }
.filename { font-size: 1.125rem; font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
.filemeta { font-size: 0.875rem; margin: 0 0 1rem; }
.entries { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; }
.entry a { display: flex; align-items: baseline; justify-content: space-between; gap: 0.75rem; padding: 0.4rem 0; text-decoration: none; color: inherit; border-bottom: 1px solid; }
.name { font-weight: 600; }
.kind, .size { font-size: 0.8125rem; opacity: 0.75; }
.empty { color: inherit; }
.notice { font-size: 0.9375rem; }
</style>
