<p align="center"><img src="frontend/public/logo.svg" alt="KittyAPI" width="88" /></p>

# KittyAPI

**AI API ゲートウェイと運用管理プラットフォーム**

KittyAPI は `kittyapi.xyz` 向けに独立して保守されているプロジェクトです。利用者は共通の API キーで設定済みのモデルを呼び出し、運用者は上流アカウント、チャネル、利用量、調達コストを一つの管理画面で管理できます。

[KittyAPI](https://kittyapi.xyz/) · [リリース](https://github.com/kittyyyds/sub2api-kitty/releases) · [不具合報告](https://github.com/kittyyyds/sub2api-kitty/issues)

[中文](README.md) · [English](README_EN.md) · 日本語

## 主な機能

| 機能 | 概要 |
| --- | --- |
| モデル接続 | OpenAI 互換の Responses、Chat Completions、ストリーミングに対応。Responses の WebSocket 利用可否はクライアントとゲートウェイの設定に応じます。 |
| アカウントとキー | 利用権限のある上流サービスを OAuth または API キーで登録し、モデルのマッピング、グループ、アカウントの振り分け、同時実行数を管理します。 |
| 利用量と課金 | モデルやグループごとに料金を設定し、リクエスト履歴、トークン利用量、費用を確認できます。 |
| チャネル運用 | チャネルの状態、リクエストの推移、費用を確認し、設定した仕入先と原価ルールから粗利益を分析できます。 |

## 利用方法、リリース、不具合報告

- **API の利用**：[KittyAPI](https://kittyapi.xyz/) で API キーを管理し、利用可能なグループとモデルを選び、サイト内の手順に従ってクライアントを設定してください。
- **ソースと更新**：このリポジトリの [リリース](https://github.com/kittyyyds/sub2api-kitty/releases) と対応するソースアーカイブを参照してください。既定ブランチと稼働中のバージョンは一致するとは限りません。他の派生版のインストーラーや成果物で上書きしないでください。
- **不具合報告**：バージョン、クライアント、エラー、機密情報を除いた再現手順を記載してください。API キー、OAuth 認証情報、顧客データは含めないでください。

## ソースの由来とライセンス

KittyAPI は [Sub2API](https://github.com/Wei-Shaw/sub2api) のオープンソースコードをベースに、独自に保守・公開しています。適用される著作権表示とライセンスは [LICENSE](LICENSE) と [サードパーティーコンポーネント](third_party/) を参照してください。利用権限のある上流サービスにのみ接続し、その利用規約を遵守してください。

Copyright (c) 2026 Wesley Liddick
