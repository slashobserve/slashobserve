{{- define "slashobserve.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "slashobserve.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "slashobserve.selectorLabels" -}}
app.kubernetes.io/name: {{ include "slashobserve.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "slashobserve.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{ include "slashobserve.selectorLabels" . }}
app.kubernetes.io/version: {{ .Values.image.tag | default .Chart.AppVersion | trunc 63 | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "slashobserve.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- .Values.serviceAccount.name | default (include "slashobserve.fullname" .) -}}
{{- else -}}
{{- .Values.serviceAccount.name | default "default" -}}
{{- end -}}
{{- end -}}

{{- define "slashobserve.image" -}}
{{- $image := printf "%s:%s" .Values.image.repository (.Values.image.tag | default .Chart.AppVersion) -}}
{{- if .Values.image.digest -}}
{{- $image = printf "%s@%s" $image .Values.image.digest -}}
{{- end -}}
{{- $image -}}
{{- end -}}

{{- define "slashobserve.datasourcesConfigMap" -}}
{{- if .Values.datasources.existingConfigMap -}}
{{- .Values.datasources.existingConfigMap -}}
{{- else if .Values.datasources.datasources -}}
{{- printf "%s-datasources" (include "slashobserve.fullname" .) -}}
{{- end -}}
{{- end -}}

{{- define "slashobserve.internalsConfigMap" -}}
{{- printf "%s-dashboards-slashobserve" (include "slashobserve.fullname" .) -}}
{{- end -}}

{{- define "slashobserve.provisionDir" -}}/provision{{- end -}}

{{- define "slashobserve.provisionFolders" -}}
{{- $folders := list -}}
{{- if .Values.provisioning.internals -}}
{{- $folders = append $folders (dict "directory" "SlashObserve" "configMap" (include "slashobserve.internalsConfigMap" .)) -}}
{{- end -}}
{{- $folders = concat $folders .Values.provisioning.folders -}}
{{- toJson $folders -}}
{{- end -}}
