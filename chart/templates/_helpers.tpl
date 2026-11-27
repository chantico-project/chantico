{{/* Non-empty when a controller, in the cluster or at alertWebhook.hostAddress, receives health alerts */}}
{{- define "chantico.alertWebhookEnabled" -}}
{{- if or .Values.alertWebhook.hostAddress (and .Values.controller.include .Values.controller.image.repository) -}}
true
{{- end -}}
{{- end -}}
