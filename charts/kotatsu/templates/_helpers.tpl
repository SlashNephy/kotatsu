{{/* fullname は、リソースの名前の前置き。 */}}
{{- define "kotatsu.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{/* labels は、すべてのリソースに付けるラベル。 */}}
{{- define "kotatsu.labels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Values.image.tag | default .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{/* selectorLabels は、ロール (component) の Pod を選ぶラベル。引数は (dict "root" $ "component" 名前)。 */}}
{{- define "kotatsu.selectorLabels" -}}
app.kubernetes.io/name: {{ .root.Chart.Name }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{/* image は、イメージの名前。引数は (dict "root" $ "ffmpeg" 真偽値)。FFmpeg が要るロールは -ffmpeg のイメージを使う。 */}}
{{- define "kotatsu.image" -}}
{{- $tag := .root.Values.image.tag | default .root.Chart.AppVersion -}}
{{- if .ffmpeg -}}
{{- printf "%s:%s-ffmpeg" .root.Values.image.repository $tag -}}
{{- else -}}
{{- printf "%s:%s" .root.Values.image.repository $tag -}}
{{- end -}}
{{- end -}}

{{/* pools は、KOTATSU_STORAGE_POOLS の値 (プール ID=マウント先 をカンマでつないだもの)。 */}}
{{- define "kotatsu.pools" -}}
{{- $pools := list -}}
{{- range .Values.storage.pools -}}
{{- $pools = append $pools (printf "%s=/var/lib/kotatsu/pools/%s" .id .id) -}}
{{- end -}}
{{- join "," $pools -}}
{{- end -}}

{{/*
env は、すべてのコンテナに渡す環境変数。
インスタンスの登録簿には、Pod の IP を載せる (設計書 1.2)。Pod のホスト名では、tenban から kyusu に届かないため。
*/}}
{{- define "kotatsu.env" -}}
- name: POD_IP
  valueFrom:
    fieldRef:
      fieldPath: status.podIP
- name: KOTATSU_SERVER_LISTEN
  value: ":8080"
- name: KOTATSU_SERVER_ADVERTISE_HOST
  value: "$(POD_IP)"
- name: KOTATSU_METRICS_LISTEN
  value: ":9090"
- name: KOTATSU_DATABASE_URL
  valueFrom:
    secretKeyRef:
      name: {{ .Values.secret.name }}
      key: {{ .Values.secret.databaseURLKey }}
- name: KOTATSU_AUTH_SECRET_KEY
  valueFrom:
    secretKeyRef:
      name: {{ .Values.secret.name }}
      key: {{ .Values.secret.secretKeyKey }}
{{- with include "kotatsu.pools" . }}
- name: KOTATSU_STORAGE_POOLS
  value: {{ . | quote }}
{{- end }}
- name: KOTATSU_KYUSU_WORK_DIR
  value: /var/lib/kotatsu/work
{{- range $name, $value := .Values.config }}
- name: {{ printf "KOTATSU_%s" $name }}
  value: {{ $value | toString | quote }}
{{- end }}
{{- end -}}

{{/*
volumeMounts は、コンテナにマウントするもの。ルートファイルシステムは読み取り専用なので、書く場所は emptyDir にする。
引数は (dict "root" $ "pools" 真偽値)。pools が true なら、録画の保存先プールもマウントする。
volume の名前は、プール ID に使える _ を含められないので、プールの並びの番号にする。
*/}}
{{- define "kotatsu.volumeMounts" -}}
- name: tmp
  mountPath: /tmp
- name: work
  mountPath: /var/lib/kotatsu/work
{{- if .pools }}
{{- range $i, $pool := .root.Values.storage.pools }}
- name: {{ printf "pool-%d" $i }}
  mountPath: {{ printf "/var/lib/kotatsu/pools/%s" $pool.id }}
{{- end }}
{{- end }}
{{- end -}}

{{/* volumes は、volumeMounts に対応する Pod の volume。引数は volumeMounts と同じ。 */}}
{{- define "kotatsu.volumes" -}}
- name: tmp
  emptyDir: {}
- name: work
  emptyDir: {}
{{- if .pools }}
{{- range $i, $pool := .root.Values.storage.pools }}
- name: {{ printf "pool-%d" $i }}
  {{- toYaml $pool.volume | nindent 2 }}
{{- end }}
{{- end }}
{{- end -}}

{{/* deployment は、ロールの Deployment。引数は (dict "root" $ "component" 名前 "args" 引数 "pod" 設定 "replicas" 台数 "ffmpeg" 真偽値 "pools" 真偽値)。 */}}
{{- define "kotatsu.deployment" -}}
{{- $root := .root -}}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ printf "%s-%s" (include "kotatsu.fullname" $root) .component }}
  labels:
    {{- include "kotatsu.labels" $root | nindent 4 }}
    app.kubernetes.io/component: {{ .component }}
spec:
  replicas: {{ .replicas }}
  selector:
    matchLabels:
      {{- include "kotatsu.selectorLabels" (dict "root" $root "component" .component) | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "kotatsu.labels" $root | nindent 8 }}
        app.kubernetes.io/component: {{ .component }}
    spec:
      {{- with $root.Values.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      securityContext:
        {{- toYaml $root.Values.podSecurityContext | nindent 8 }}
      terminationGracePeriodSeconds: {{ .pod.terminationGracePeriodSeconds }}
      containers:
        - name: kotatsu
          image: {{ include "kotatsu.image" (dict "root" $root "ffmpeg" .ffmpeg) }}
          imagePullPolicy: {{ $root.Values.image.pullPolicy }}
          args:
            {{- toYaml .args | nindent 12 }}
          env:
            {{- include "kotatsu.env" $root | nindent 12 }}
          ports:
            - name: http
              containerPort: 8080
            - name: metrics
              containerPort: 9090
          livenessProbe:
            httpGet:
              path: /healthz
              port: http
          readinessProbe:
            httpGet:
              path: /readyz
              port: http
          securityContext:
            {{- toYaml $root.Values.securityContext | nindent 12 }}
          resources:
            {{- toYaml .pod.resources | nindent 12 }}
          volumeMounts:
            {{- include "kotatsu.volumeMounts" (dict "root" $root "pools" .pools) | nindent 12 }}
      volumes:
        {{- include "kotatsu.volumes" (dict "root" $root "pools" .pools) | nindent 8 }}
      {{- with .pod.nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .pod.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .pod.affinity }}
      affinity:
        {{- toYaml . | nindent 8 }}
      {{- end }}
{{- end -}}
