{{- define "spifferize.admission_controller.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "spifferize.admission_controller.app_label" -}}
app
{{- end -}}