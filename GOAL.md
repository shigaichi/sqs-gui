## 要件定義

AWS SQSのローカルにおけるクライアントを作成する。  
実際のAWSにキューを作成した場合はAWSのコンソール画面で確認すればよいが、ElasticMQ、LocalStackなどをつかってローカルにSQSを作成した場合は画面がないため不便である。  
AWS CLIで操作するしかない。  
[kobim/sqs-insight](https://github.com/kobim/sqs-insight)
など既存のツールはあるがこれはメッセージの閲覧、キューの一覧などの確認しかできない。キューの作成、メッセージの送信などができず不便である。  
全機能は無理だが、動作確認に必要な主要な機能は実装したい。基本的にAWSコンソールの本物を模倣する。

ローカルでの動作確認用なので認証は不要

画面はすべて英語で多言語対応は不要

## 基本設計

アプリケーションはSQSへの接続に必要な情報を環境変数経由で受取り、SQSに接続する。基本的にLocalStackなどのローカルのSQSを念頭に置くが、本物のAWS
SQSにも接続しようと思えばできるようにする。

画面は次のようなものが必要。

### キュー一覧画面

キュー一覧。AWSでは以下のような情報を閲覧可能。

* Name
* Type
* Created
* Mesages Availavle
* Messages in flight
* Encryption
* Content-based deduplication

多すぎればページングする。  
Nameをクリックすると各キューのページに遷移する。  
検索して絞り込む機能をつける。

Create queueへの遷移ボタンもある。

### 各キューの個別ページ

本物AWSには様々な機能があるが一部だけあれば良い。

* 名前などの各種情報を表示
* キュー削除ボタン
* Purgeボタン（メッセージの全削除）
* Editボタン
    * Editページへ遷移
* Send and receive messagesボタン
    * Send and receive messagesページへ遷移

### Send and receive messagesページ

メッセージの送受信を行う

#### Send message 機能

以下を設定可能。送信ボタンを押すと送信する

* Message body
* Message group ID
* Delivery delay
* Message attributes

#### Receive messages 機能

Poll for messagesボタンを押下するとポーリングする。
受信したメッセージの閲覧が可能。

### Editページ

作成済みのキューの設定の変更が可能。

### Create queue

キューの新規作成が可能。

## アーキテクチャ

Goファイルは基本的にinternalフォルダに作成する。その中をさらにフォルダで仕切ることはしない。

handlerはserviceを呼び出し、serviceはrepositoryを呼び出します。  
serviceにビジネスロジック、レポジトリにSQSアクセス処理を実装します。

### ページの追加

ページを追加する際はフロントエンド関連について

* templates/pagesにgohtmlを追加
* assetsにtsを追加
* vite.config.tsのinputに追加したtsを追記

が必要です。tsは空でも良いので追加してください。

### 技術背景

* webフレームワークは使いません
* 例外処理はスタックトレースが必要なためgithub.com/cockroachdb/errorsを使います
* ログ出力はslogを使います（print、logをつかっている場所は要修正）
* 単体試験は一旦不要です
* フロントエンドはtailwindcssでスタイリングします
* 画面はモバイルデバイスを考慮する必要はありません

### コーディング方針の補足

* コード内のコメント、ログ、エラーメッセージは英語で記述する（TODO、FIXMEは将来削除するため日本語のままでも良い）
* markdownファイルは日本語のままで良い

## 要確認点

- 現時点で特になし
