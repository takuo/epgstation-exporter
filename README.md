# epgstation-exporter

[EPGStation](https://github.com/l3tnun/EPGStation) の Prometheus エクスポーター。

[ルール名拡張版](https://github.com/takuo/EPGStation/commit/f600916f7d3b4a7d3a5da2f6414a3da307e82ef6)に対応しているので、そちらを使用している場合はルールメトリクスがわかりやすくルール名で表示されます。(ruleName -> Keyword -> IDの順にfallback)

## メトリクス

| メトリクス名 | 種別 | ラベル | 説明 |
|---|---|---|---|
| `epgstation_up` | Gauge | - | EPGStation が稼働中かどうか (1: 正常, 0: ダウン) |
| `epgstation_info` | Gauge | `version` | EPGStation のバージョン情報 |
| `epgstation_reserves_total` | Gauge | `type` | 予約の総数 |
| `epgstation_recording_total` | Gauge | - | 録画中の番組数 |
| `epgstation_storage_available_bytes` | Gauge | `name` | ストレージの空き容量 (バイト) |
| `epgstation_storage_used_bytes` | Gauge | `name` | ストレージの使用量 (バイト) |
| `epgstation_storage_total_bytes` | Gauge | `name` | ストレージの総容量 (バイト) |
| `epgstation_encode_running_total` | Gauge | - | 実行中のエンコードジョブ数 |
| `epgstation_encode_waiting_total` | Gauge | - | 待機中のエンコードジョブ数 |
| `epgstation_streams_total` | Gauge | `type` | ストリームの総数 |
| `epgstation_rules_total` | Gauge | - | 録画ルールの総数 |
| `epgstation_rule_reserves_total` | Gauge | `id`, `name` | ルールごとの予約数 |

ストレージメトリクスは `--no-enable-storage`、ストリームメトリクスは `--no-enable-streams` で無効化できます。

## 使い方

### バイナリ

```sh
epgstation-exporter --api-url http://localhost:8888/api
```

### Docker

```sh
docker run -d \
  -p 9888:9888 \
  -e EPGSTATION_API_URL=http://epgstation:8888/api \
  ghcr.io/takuo/epgstation-exporter:latest
```

### Docker Compose

```sh
docker compose up -d
```

## オプション

| フラグ | 環境変数 | デフォルト | 説明 |
|---|---|---|---|
| `--api-url` | `EPGSTATION_API_URL` | `http://localhost:8888/api` | EPGStation API の URL |
| `--port` | `EPGSTATION_EXPORTER_PORT` | `9888` | リッスンポート |
| `--metrics-path` | `EPGSTATION_METRICS_PATH` | `/metrics` | メトリクスのパス |
| `--[no-]enable-storage` | `EPGSTATION_ENABLE_STORAGE` | `true` | ストレージメトリクスの有効/無効 |
| `--[no-]enable-streams` | `EPGSTATION_ENABLE_STREAMS` | `true` | ストリームメトリクスの有効/無効 |

## Grafana ダッシュボード

`grafana/dashboard.json` に Grafana ダッシュボードが含まれています。

**インポート方法:**

1. Grafana UI で **Dashboards → Import** を開く
2. [grafana/dashboard.json](https://raw.githubusercontent.com/takuo/epgstation-exporter/refs/heads/main/grafana/dashboard.json) の内容を貼り付けるか、ファイルをアップロード
3. Prometheus データソースを選択して **Import**

**パネル一覧:**

| セクション | パネル |
|---|---|
| Overview | Status, Version, Recording, Rules, Encode Running, Encode Waiting |
| Reserves | Reserves by Type (normal / conflicts / skips / overlaps) |
| Rules | Reserves per Rule (Top 20) |
| Storage | Storage Usage (%), Storage Total / Used / Available |
| Streams | Streams by Type |

## ビルド

```sh
go build ./cmd/epgstation-exporter
```

## ライセンス

MIT
