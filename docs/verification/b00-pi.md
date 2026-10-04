# B00 · Pi 版本验证

日期：2026-10-04

- 模块：`github.com/sllt/pi v0.4.2-0.20260926071721-4c292d04b898`
- 该版本由 `go get github.com/sllt/pi@4c292d04b898d95862ea74e17be7adb115a4ad54` 解析，对应设计查阅提交。
- 使用的 API：`pi.Build`、`WithExplicitHTTPStatus`、`WithConfig`、`WithResource`（Owned / Borrowed）、`App.HTTPHandler`、`App.GET`、`App.Go`、`App.Start`、`App.Stop`、`response.Raw`、`response.Stream`。
- 未使用文档未核实的 `app.RunJob`、`app.Sandbox` 或 `WithManagedSQL`。
- HTTP 监听由宿主 `http.Server` 持有，Pi 的 HTTP/gRPC/metrics 监听关闭，避免双重监听。
- `go test ./internal/bootstrap` 通过：handler 与 SSE 可编译运行；SSE 正文没有补 JSON 错误；资源启动失败会关闭已启动资源；停止后不能再次启动；Borrowed 不调用 Start/Stop；worker 内部消化的失败不会返回给 Pi。
