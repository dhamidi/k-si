<!-- field.vue — one settings.Field rendered by its Kind (docs/16, decision-020):
     the generalised control the settings form loops once per field. It is
     request_field.vue generalised — keeping text/longtext/choice/file/secret and
     adding number — but keyed on Field.Kind, not the Flow C FieldView.Type.
     A secret renders masked (type=password) and NEVER echoes its value back into
     the page (decision-004); a file field likewise carries no value. field.Error
     shows when a parse produced an error.

     TODO(phase 4): unify request_field.vue into field.vue. Flow C still renders
     through request_field.vue (its FieldView carries .Type/.Required, not .Kind),
     so the two controls stay separate until that repoint is proven green. -->
<template>
	<div class="field">
		<label :for="field.Name">{{ field.Label }}</label>

		<input v-if="field.Kind == 'text'" type="text"
			:id="field.Name" :name="field.Name" :value="field.Value">

		<textarea v-if="field.Kind == 'longtext'" rows="4"
			:id="field.Name" :name="field.Name">{{ field.Value }}</textarea>

		<select v-if="field.Kind == 'choice'" :id="field.Name" :name="field.Name">
			<option v-for="opt in field.Options" :value="opt" :selected="opt == field.Value">{{ opt }}</option>
		</select>

		<input v-if="field.Kind == 'number'" type="number"
			:id="field.Name" :name="field.Name" :value="field.Value">

		<input v-if="field.Kind == 'bool'" type="checkbox" value="true"
			:id="field.Name" :name="field.Name" :checked="field.Value == 'true'">

		<input v-if="field.Kind == 'secret'" type="password" autocomplete="off"
			:id="field.Name" :name="field.Name">

		<input v-if="field.Kind == 'file'" type="file"
			:id="field.Name" :name="field.Name">

		<p class="error" v-if="field.Error">{{ field.Error }}</p>
	</div>
</template>

<style scoped>
.field { display: flex; flex-direction: column; gap: 0.25rem; }
label { font-weight: 600; }
input, textarea, select { padding: 0.4rem; font: inherit; }
.error { color: #b00020; margin: 0; font-size: 0.875rem; }
</style>
