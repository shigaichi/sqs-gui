# sqs-gui

## 開発方法

```sh
pnpm run dev
```

```sh
export DEV_MODE=true
export AWS_SQS_ENDPOINT=http://localhost:9324
go run cmd/main.go
```

CLIによるキュー一覧確認方法

```sh
aws --endpoint-url=http://localhost:9324 sqs list-queues
```
