<!-- setting_form.vue — one setting's <form> (docs/16, decision-020): a control per
     field (looped through field.vue), then the submit. When the setting is DYNAMIC
     (its ToForm carries an Update — the initiator allowlist) each row gains a
     Remove button and the list gains an Add button; both are submit buttons whose
     formaction is the reshape route, so they POST the WHOLE form (every current
     value rides along) and only the shape-changing op/index differs. A flat setting
     renders exactly one Save.

     This component is rendered TWICE from the one template: embedded in the full
     page (view_setting.vue, via v-html of the turbo-frame-wrapped output) and alone
     as the reshape fragment. The <turbo-frame> wrapper is applied at the render edge
     in Go, because htmlc reads a hyphenated tag as a component reference and cannot
     emit <turbo-frame> itself (docs/16). Works without JavaScript: absent Turbo, a
     reshape button does a full-page POST that re-renders with values preserved. -->
<template>
	<form method="post" :action="setting.SavePath">
		<div class="setting-row" v-for="row in setting.Fields">
			<field :field="row"></field>
			<button v-if="setting.Dynamic" type="submit" class="remove"
				:formaction="setting.ReshapePath" name="remove" :value="row.Index">Remove</button>
		</div>

		<div v-if="setting.Dynamic" class="reshape-add">
			<button type="submit" :formaction="setting.ReshapePath" name="add" value="1">Add address</button>
		</div>

		<button type="submit" class="save">Save</button>
	</form>
</template>

<style scoped>
form { display: flex; flex-direction: column; gap: 1rem; }
.setting-row { display: flex; align-items: flex-end; gap: 0.5rem; }
.setting-row > .field { flex: 1; }
.remove { padding: 0.4rem 0.75rem; align-self: stretch; }
.reshape-add button, .save { align-self: flex-start; padding: 0.5rem 1rem; }
.save { margin-top: 0.5rem; }
</style>
