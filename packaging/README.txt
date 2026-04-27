ChatGpt Image Studio

1. 首次启动时，程序会自动在 data/config.toml 生成本地配置文件。
2. 主要需要关注的配置项：
   - app.auth_key
   - app.api_key
   - sync.enabled / sync.base_url / sync.management_key
   - cpa.base_url / cpa.api_key
3. 默认访问地址：
   - 多人门户: http://127.0.0.1:7000
   - 后台管理: http://127.0.0.1:7000/admin
   - 第三方图片 API Base URL: http://127.0.0.1:7000
4. OpenAI 兼容图片接口地址未变化：
   - POST /v1/images/generations
   - POST /v1/images/edits
   - GET /v1/models
5. 如果启动失败，请查看：
   - data/last-startup-error.txt
6. 如果网页打不开，请确认 static/index.html 存在且未被删除。
