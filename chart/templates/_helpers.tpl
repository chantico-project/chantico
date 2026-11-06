{{- define "chantico.dataClaimName" -}}
{{- .Values.persistence.existingClaim | default "chantico-persistent-volume-claim" -}}
{{- end }}

{{/* include "chantico.pvc" (dict "root" . "name" "chantico-prometheus-data" "persistence" .Values.prometheus.persistence) */}}
{{- define "chantico.pvc" -}}
{{- if and .persistence.enabled (not .persistence.existingClaim) }}
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: {{ .name }}
  namespace: {{ .root.Release.Namespace }}
spec:
  accessModes:
    {{- toYaml .persistence.accessModes | nindent 4 }}
  {{- with .persistence.storageClass }}
  storageClassName: {{ . | quote }}
  {{- end }}
  {{- with .persistence.volumeName }}
  volumeName: {{ . }}
  {{- end }}
  resources:
    requests:
      storage: {{ .persistence.size | quote }}
{{- end }}
{{- end }}

{{- define "chantico.volumeSource" -}}
{{- if .persistence.enabled -}}
persistentVolumeClaim:
  claimName: {{ .persistence.existingClaim | default .name }}
{{- else -}}
emptyDir: {}
{{- end }}
{{- end }}

{{/* Must match volumes.DataVolumeLabel, which the controller sets on generator jobs. */}}
{{- define "chantico.dataVolumeLabel" -}}
chantico-project.github.io/data-volume: {{ include "chantico.dataClaimName" . | quote }}
{{- end }}

{{- define "chantico.dataVolumeAffinity" -}}
{{- if has "ReadWriteOnce" .Values.persistence.accessModes -}}
affinity:
  podAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchLabels:
            {{- include "chantico.dataVolumeLabel" . | nindent 12 }}
        topologyKey: kubernetes.io/hostname
{{- end }}
{{- end }}