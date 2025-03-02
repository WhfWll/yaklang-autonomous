# Yaklang 爬虫模块

## 概述

Yaklang 爬虫模块是一个高度可配置的网站爬行工具，用于自动发现和分析网站结构、内容和潜在的安全问题。它支持多种配置选项，可以根据需要调整爬行行为，如深度限制、域名限制、请求数量限制等。

## 核心工作原理

### 爬行思路

1. **初始化**：从用户提供的起始 URL 开始，创建初始请求。
2. **广度优先爬行**：使用并发处理请求，按照广度优先的方式爬行网站。
3. **内容解析**：解析 HTML、JavaScript 等内容，提取链接、表单和其他可能的请求。
4. **智能过滤**：根据配置的规则（域名、路径、后缀等）过滤不需要访问的 URL。
5. **会话管理**：维护 Cookie 和其他会话信息，支持自动登录和表单提交。
6. **重定向处理**：智能处理 HTTP 重定向，保持会话状态。

### 主要组件

1. **Crawler**：爬虫的核心控制器，管理请求队列和爬行状态。
2. **Req**：表示一个 HTTP 请求及其响应，包含请求/响应数据和元数据。
3. **Config**：存储爬虫配置，如并发数、超时时间、过滤规则等。
4. **PageInformationWalker**：解析页面内容，提取有用信息。

## 爬行流程

1. **创建爬虫实例**：通过 `NewCrawler(urls string, opts ...ConfigOpt)` 函数创建爬虫实例，设置初始 URL 和配置选项。
   - 解析输入的 URL 列表 (`utils.PrettifyListFromStringSplited`, `utils.ParseStringToUrlsWith3W`)
   - 初始化配置对象 (`Config.init()`)
   - 应用用户提供的配置选项 (`opt(config)`)
   - 创建爬虫实例并初始化各种同步原语和通道

2. **启动爬行**：调用 `Crawler.Run()` 方法开始爬行过程。
   - 启动两个并行协程：一个用于提交初始请求，一个用于处理请求队列
   - 在 `startUpSubmitTask` 协程中，通过 `createReqFromUrl` 创建初始请求并提交到请求队列
   - 在主处理协程中，调用 `run()` 方法开始处理请求队列

3. **请求队列处理**：在 `Crawler.run()` 方法中循环处理请求队列。
   - 从 `reqChan` 通道接收请求
   - 调用 `preReq(r)` 进行请求预处理：
     - 检查深度限制 (`r.depth > config.maxDepth`)
     - 添加请求头部 (`r.request.Header.Set`)
     - 设置基础认证 (`r.request.SetBasicAuth`)
     - 添加 User-Agent (`r.request.Header.Set("User-Agent", config.userAgent)`)
     - 设置 Cookie (`r.request.AddCookie`)
     - 验证 URL 后缀是否允许访问
   - 检查请求是否已处理过 (`requestedHash.Load(r.Hash())`)
   - 检查 URL 是否符合访问规则 (`config.CheckShouldBeHandledURL(r.request.URL)`)
   - 创建协程执行请求 (`execReq(r)`)

4. **执行请求**：在 `Crawler.execReq(r)` 方法中发送 HTTP 请求并处理响应。
   - 获取配置的 HTTP 选项 (`config.GetLowhttpConfig()`)
   - 添加重定向处理选项 (`lowhttp.WithRedirectHandler`)
   - 发送 HTTP 请求 (`lowhttp.HTTP(opts...)`)
   - 解析响应 (`utils.ReadHTTPResponseFromBytes`)
   - 分离响应头和体 (`lowhttp.SplitHTTPPacketFast`)
   - 检查 MIME 类型 (`mime.ParseMediaType`, `utils.MatchAnyOfGlob`)
   - 调用请求回调函数 (`config.onRequest(r)`)

5. **内容分析**：在 `Crawler.handleReqResult(r)` 方法中分析响应内容。
   - 使用 `PageInformationWalker` 解析页面内容：
     - 提取 JavaScript 内容 (`WithFetcher_JavaScript`)
     - 提取 HTML 标签 (`WithFetcher_HtmlTag`)
     - 处理表单 (`CreateReqHTMLFormData`)
     - 提取链接 (`NewHTTPRequest`)
   - 处理 JavaScript 内容：
     - 获取外部 JavaScript 文件 (`lowhttp.HTTP`)
     - 解析 JavaScript 代码 (`HandleJSGetNewRequest`)
     - 提取 API 调用和 URL (`handleJS`)

6. **处理新发现的 URL**：在 `handleReqResultEx` 函数中处理从页面中提取的 URL。
   - 存储表单请求 (`foundFormRequests.Store`)
   - 存储发现的 URL (`foundPathOrUrls.Store`)
   - 对每个表单请求调用 `reqHandler`
   - 对每个 URL 调用 `urlHandler`

7. **提交新请求**：通过 `Crawler.submit(r)` 方法将新请求添加到队列。
   - 增加等待组计数 (`reqWaitGroup.Add(1)`)
   - 将请求发送到请求通道 (`reqChan <- r`)

8. **结果处理**：通过用户提供的回调函数处理爬行结果。
   - 在 `WithOnRequest` 配置选项中设置回调函数
   - 在 `execReq` 方法结束时调用回调函数

## 特色功能

1. **智能表单处理**：自动识别和处理表单，包括登录表单和上传表单。
2. **JavaScript 解析**：支持解析 JavaScript 代码，提取其中的 URL 和 API 调用。
3. **Cookie 管理**：自动管理和维护 Cookie，支持会话保持。
4. **并发控制**：可配置的并发数，平衡性能和服务器负载。
5. **深度限制**：控制爬行深度，避免无限爬行。
6. **域名限制**：可以限制爬行范围在特定域名内。
7. **MIME 类型过滤**：可以过滤特定 MIME 类型的响应，如图片、视频等。

## 使用示例

```go
// 创建爬虫实例
crawler, err := NewCrawler(
    "https://example.com",
    WithMaxDepth(3),
    WithConcurrent(10),
    WithFixedCookie("session", "value"),
    WithOnRequest(func(req *Req) {
        fmt.Println("访问URL:", req.Url())
    }),
)
if err != nil {
    log.Fatal(err)
}

// 启动爬行
err = crawler.Run()
if err != nil {
    log.Fatal(err)
}