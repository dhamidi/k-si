<!-- request_field.vue — one spec field: label + a control chosen by type + error
     (decision-005). Renders in isolation from a single FieldView; the page loops
     it once per field. Semantic and labelled; the `required` attribute mirrors
     the spec so the browser enforces it before submit. -->
<template>
	<div class="request-field">
		<label :for="field.Name">{{ field.Label }}</label>

		<input v-if="field.Type == 'text'" type="text"
			:id="field.Name" :name="field.Name" :value="field.Value" :required="field.Required">

		<textarea v-if="field.Type == 'longtext'" rows="4"
			:id="field.Name" :name="field.Name" :required="field.Required">{{ field.Value }}</textarea>

		<select v-if="field.Type == 'choice'" :id="field.Name" :name="field.Name" :required="field.Required">
			<option v-for="opt in field.Options" :value="opt" :selected="opt == field.Value">{{ opt }}</option>
		</select>

		<input v-if="field.Type == 'file'" type="file"
			:id="field.Name" :name="field.Name" :required="field.Required">

		<!-- secret: masked, never carries a Value back into the page (decision-004) -->
		<input v-if="field.Type == 'secret'" type="password" autocomplete="off"
			:id="field.Name" :name="field.Name" :required="field.Required">

		<p class="error" v-if="field.Error">{{ field.Error }}</p>
	</div>
</template>

<style scoped>
.request-field { display: flex; flex-direction: column; gap: 0.25rem; }
label { font-weight: 600; }
input, textarea, select { padding: 0.4rem; font: inherit; }
.error { margin: 0; font-size: 0.875rem; }
</style>
