# pnpm audit --json の結果を、Code scanning に送る SARIF に直す (.github/workflows/scan.yml)。
# pnpm audit は SARIF を出さないので、ここで直す。
#
#   jq --rawfile lock frontend/pnpm-lock.yaml -f .github/scan/pnpm-audit-sarif.jq audit.json
#
# 規則は勧告 (GHSA) ごと、結果は勧告に当たったパッケージの版ごとに作る。
# 結果の位置は、frontend/pnpm-lock.yaml の packages の節で、その版を書いた行にする。見つからなければ 1 行目にする。
# packages の節は snapshots の節より前にあるので、最初に当たった行を使う。

# score は、Code scanning が重大度の分類に使う security-severity。GitHub の区切り (9.0 以上が critical、7.0 以上が high、
# 4.0 以上が medium) に収まる値にする。
def score:
  {critical: "9.5", high: "8.0", moderate: "5.5", low: "2.0", info: "0.0"}[.] // "0.0";

# level は、SARIF の結果の重さ。
def level:
  if . == "critical" or . == "high" then "error"
  elif . == "moderate" then "warning"
  else "note"
  end;

($lock | split("\n")) as $lines
| [.advisories[]] as $advisories
| {
    "$schema": "https://json.schemastore.org/sarif-2.1.0.json",
    version: "2.1.0",
    runs: [
      {
        tool: {
          driver: {
            name: "pnpm audit",
            informationUri: "https://pnpm.io/cli/audit",
            rules: [
              $advisories[]
              | {
                  id: .github_advisory_id,
                  name: .module_name,
                  shortDescription: { text: .title },
                  fullDescription: {
                    text: "\(.module_name) \(.vulnerable_versions) (patched: \(.patched_versions))"
                  },
                  helpUri: .url,
                  help: { text: "\(.title)\n\(.url)" },
                  properties: {
                    "security-severity": (.severity | score),
                    tags: ["security", "dependency"]
                  }
                }
            ]
            # SARIF では規則の id が重なってはいけない。同じ GHSA の勧告が複数あれば、1 つにまとめる。
            | unique_by(.id)
          }
        },
        results: [
          $advisories[] as $advisory
          | $advisory.findings[]
          | . as $finding
          | "\($advisory.module_name)@\($finding.version)" as $key
          # スコープ付きのパッケージ (@scope/name) は、YAML で @ から始められないので、引用符で囲んで書かれる。
          | ([$lines | to_entries[] | select(.value == "  \($key):" or .value == "  '\($key)':") | .key + 1][0] // 1) as $line
          | {
              ruleId: $advisory.github_advisory_id,
              level: ($advisory.severity | level),
              message: {
                text: "\($advisory.module_name)@\($finding.version): \($advisory.title)\n\($finding.paths | join("\n"))"
              },
              locations: [
                {
                  physicalLocation: {
                    artifactLocation: { uri: "frontend/pnpm-lock.yaml" },
                    region: { startLine: $line }
                  }
                }
              ],
              # 同じ勧告に複数の版が当たっても、別のアラートとして残す。
              partialFingerprints: {
                package: "\($advisory.module_name)@\($finding.version)"
              }
            }
        ]
      }
    ]
  }
