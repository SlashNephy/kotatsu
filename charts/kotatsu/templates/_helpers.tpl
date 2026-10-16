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

{{/*
image は、イメージの名前。引数は (dict "root" $ "ffmpeg" 真偽値)。FFmpeg が要るロールは -ffmpeg のイメージを使う。
digest を指定していれば、タグに加えて digest で固定する。
*/}}
{{- define "kotatsu.image" -}}
{{- $values := .root.Values.image -}}
{{- $tag := $values.tag | default .root.Chart.AppVersion -}}
{{- $name := printf "%s:%s" $values.repository $tag -}}
{{- $digest := $values.digest -}}
{{- if .ffmpeg -}}
{{- $name = printf "%s:%s-ffmpeg" $values.repository $tag -}}
{{- $digest = $values.ffmpegDigest -}}
{{- end -}}
{{- if $digest -}}
{{- printf "%s@%s" $name $digest -}}
{{- else -}}
{{- $name -}}
{{- end -}}
{{- end -}}

{{/* secretRef は、Secret のキーから値を読む環境変数の valueFrom。引数は (dict "name" Secret の名前 "key" キー)。 */}}
{{- define "kotatsu.secretRef" -}}
valueFrom:
  secretKeyRef:
    name: {{ .name }}
    key: {{ .key }}
{{- end -}}

{{/*
trustedProxies は、KOTATSU_SERVER_TRUSTED_PROXIES の値 (ingress.trustedProxies をカンマでつないだもの)。
Ingress を使うのに、信頼するプロキシをどこにも設定していなければ、全リクエストが 400 になるので、描画を止める。
config.SERVER_TRUSTED_PROXIES でも渡せるが、両方に書くと同じ環境変数が 2 つになるので、どちらか一方にさせる。
*/}}
{{- define "kotatsu.trustedProxies" -}}
{{- $fromConfig := hasKey .Values.config "SERVER_TRUSTED_PROXIES" -}}
{{- if and .Values.ingress.trustedProxies $fromConfig -}}
{{- fail "set either ingress.trustedProxies or config.SERVER_TRUSTED_PROXIES, not both" -}}
{{- end -}}
{{- if and .Values.ingress.enabled (not .Values.ingress.trustedProxies) (not $fromConfig) -}}
{{- fail "ingress.trustedProxies is required when ingress.enabled is true; set the CIDR of the ingress controller's Pods (e.g. the Pod network), or every request through the ingress is rejected with 400" -}}
{{- end -}}
{{- join "," .Values.ingress.trustedProxies -}}
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
{{- with include "kotatsu.trustedProxies" . }}
- name: KOTATSU_SERVER_TRUSTED_PROXIES
  value: {{ . | quote }}
{{- end }}
- name: KOTATSU_DATABASE_URL
  {{- include "kotatsu.secretRef" .Values.secrets.databaseURL | nindent 2 }}
- name: KOTATSU_AUTH_SECRET_KEY
  {{- include "kotatsu.secretRef" .Values.secrets.secretKey | nindent 2 }}
{{- with include "kotatsu.pools" . }}
- name: KOTATSU_STORAGE_POOLS
  value: {{ . | quote }}
{{- end }}
{{- if .Values.storage.sidecar.volume }}
- name: KOTATSU_STORAGE_SIDECAR_DIR
  value: /var/lib/kotatsu/sidecar
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
{{- if .root.Values.storage.sidecar.volume }}
- name: sidecar
  mountPath: /var/lib/kotatsu/sidecar
{{- end }}
{{- end }}
{{- end -}}

{{/* volumes は、volumeMounts に対応する Pod の volume。引数は volumeMounts と同じものに、作業ディレクトリの容量の上限 "workSizeLimit" (空なら上限なし) を足す。 */}}
{{- define "kotatsu.volumes" -}}
- name: tmp
  emptyDir: {}
- name: work
  {{- with .workSizeLimit }}
  emptyDir:
    sizeLimit: {{ . }}
  {{- else }}
  emptyDir: {}
  {{- end }}
{{- if .pools }}
{{- range $i, $pool := .root.Values.storage.pools }}
- name: {{ printf "pool-%d" $i }}
  {{- toYaml $pool.volume | nindent 2 }}
{{- end }}
{{- with .root.Values.storage.sidecar.volume }}
- name: sidecar
  {{- toYaml . | nindent 2 }}
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
      {{- with .pod.priorityClassName | default $root.Values.priorityClassName }}
      priorityClassName: {{ . }}
      {{- end }}
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
        {{- include "kotatsu.volumes" (dict "root" $root "pools" .pools "workSizeLimit" .pod.workSizeLimit) | nindent 8 }}
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
